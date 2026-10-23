//go:build !windows

package core

// Linux system proxy: GNOME/gsettings (the de-facto system-wide proxy
// settings on GNOME and GTK-based desktops; read by libgsettings consumers).
// Best effort by design: on a desktop without gsettings the feature reports
// unsupported instead of pretending.

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

func gsettings(args ...string) (string, error) {
	out, err := exec.Command("gsettings", args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func gsettingsAvailable() bool {
	_, err := exec.LookPath("gsettings")
	return err == nil
}

func SetSystemProxy(on bool, addr string) error {
	if !gsettingsAvailable() {
		return errors.New("системный прокси на этой системе не поддерживается (нужен GNOME/gsettings)")
	}
	if !on {
		if _, err := gsettings("set", "org.gnome.system.proxy", "mode", "none"); err != nil {
			return fmt.Errorf("gsettings: %w", err)
		}
		return nil
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("адрес прокси %q: %v", addr, err)
	}
	if _, err := gsettings("set", "org.gnome.system.proxy", "mode", "manual"); err != nil {
		return fmt.Errorf("gsettings: %w", err)
	}
	// Loopback and LAN stay direct; everything else goes through us.
	ignore := "['localhost', '127.0.0.0/8', '::1', '10.0.0.0/8', '172.16.0.0/12', '192.168.0.0/16']"
	for _, scheme := range []struct{ schema string }{
		{"org.gnome.system.proxy.http"},
		{"org.gnome.system.proxy.https"},
	} {
		if _, err := gsettings("set", scheme.schema, "use-same-proxy", "true"); err != nil {
			return fmt.Errorf("gsettings: %w", err)
		}
		if _, err := gsettings("set", scheme.schema, "enable-proxy", "true"); err != nil {
			return fmt.Errorf("gsettings: %w", err)
		}
		if _, err := gsettings("set", scheme.schema, "host", host); err != nil {
			return fmt.Errorf("gsettings: %w", err)
		}
		if _, err := gsettings("set", scheme.schema, "port", port); err != nil {
			return fmt.Errorf("gsettings: %w", err)
		}
	}
	if _, err := gsettings("set", "org.gnome.system.proxy", "ignore-hosts", ignore); err != nil {
		return fmt.Errorf("gsettings: %w", err)
	}
	return nil
}

func SystemProxySupported() bool { return gsettingsAvailable() }

// ClearStaleSystemProxy disables the system proxy when it points at our own
// HTTP port — the mark of a previous run that died without teardown.
func ClearStaleSystemProxy(localPort int) bool {
	if !gsettingsAvailable() {
		return false
	}
	mode, err := gsettings("get", "org.gnome.system.proxy", "mode")
	if err != nil || strings.TrimSpace(mode) != "'manual'" {
		return false
	}
	host, err := gsettings("get", "org.gnome.system.proxy.http", "host")
	if err != nil {
		return false
	}
	port, err := gsettings("get", "org.gnome.system.proxy.http", "port")
	if err != nil {
		return false
	}
	h := strings.Trim(strings.TrimSpace(host), "'\"")
	if h != "127.0.0.1" && h != "localhost" && h != "::1" {
		return false
	}
	if strings.TrimSpace(port) != strconv.Itoa(localPort) {
		return false
	}
	return SetSystemProxy(false, "") == nil
}
