//go:build windows

package core

// Windows system proxy: HKCU registry + a WinINET refresh broadcast so
// running applications pick the change without a logoff. User-scope only —
// no admin rights involved.

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

var (
	wininet           = syscall.NewLazyDLL("wininet.dll")
	procInternetSetOp = wininet.NewProc("InternetSetOptionW")
)

const (
	internetOptionSettingsChanged = 39
	internetOptionRefresh         = 37
)

func refreshWinINET() error {
	r1, _, err := procInternetSetOp.Call(0, uintptr(internetOptionSettingsChanged), 0, 0)
	if r1 == 0 {
		return fmt.Errorf("InternetSetOption(SETTINGS_CHANGED): %v", err)
	}
	r1, _, err = procInternetSetOp.Call(0, uintptr(internetOptionRefresh), 0, 0)
	if r1 == 0 {
		return fmt.Errorf("InternetSetOption(REFRESH): %v", err)
	}
	return nil
}

// SetSystemProxy enables (addr = "127.0.0.1:port") or disables the
// per-user HTTP proxy.
func SetSystemProxy(on bool, addr string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("registry: %w", err)
	}
	defer k.Close()
	if !on {
		if err := k.SetDWordValue("ProxyEnable", 0); err != nil {
			return err
		}
		if err := refreshWinINET(); err != nil {
			return err
		}
		return nil
	}
	if err := k.SetStringValue("ProxyServer", addr); err != nil {
		return err
	}
	if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
		return err
	}
	// Keep loopback and LAN direct; everything else goes through us.
	excl := []string{"localhost", "127.*", "10.*", "192.168.*", "<local>"}
	for i := 16; i <= 31; i++ {
		excl = append(excl, fmt.Sprintf("172.%d.*", i))
	}
	_ = k.SetStringValue("ProxyOverride", strings.Join(excl, ";"))
	if err := refreshWinINET(); err != nil {
		return err
	}
	return nil
}

func SystemProxySupported() bool { return true }

// ClearStaleSystemProxy disables the system proxy when it points at our own
// local HTTP port — the footprint of a previous run that died without
// teardown (crash, Task Manager kill), leaving browsers routed into a dead
// port. The host part is matched loosely (127.0.0.1 or the 0.0.0.0 the
// bridge binds with listen_all); foreign proxy settings are left untouched.
// Reports whether a stale entry was cleared.
func ClearStaleSystemProxy(localPort int) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	enable, _, err := k.GetIntegerValue("ProxyEnable")
	if err != nil || enable == 0 {
		return false
	}
	server, _, err := k.GetStringValue("ProxyServer")
	if err != nil {
		return false
	}
	host, port, err := net.SplitHostPort(server)
	if err != nil || port != strconv.Itoa(localPort) {
		return false
	}
	switch host {
	case "127.0.0.1", "localhost", "::1", "0.0.0.0", "::":
	default:
		return false
	}
	return SetSystemProxy(false, "") == nil
}
