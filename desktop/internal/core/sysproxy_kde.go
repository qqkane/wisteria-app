//go:build !windows

package core

// KDE system proxy backend: kioslaverc, written via kwriteconfig and read
// by KIO (Plasma 5/6). Plasma's KConfigWatcher picks the change up live in
// modern versions; older KIO applications read the file on their next start
// — acceptable best effort, same class as the gsettings backend.

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

// ktool finds the newest installed kwriteconfig/kreadconfig of a kind.
func ktool(kind string) string {
	for _, v := range []string{"6", "5", ""} {
		name := kind + v
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

func kdeBackend() (proxyBackend, bool) {
	if ktool("kwriteconfig") == "" {
		return proxyBackend{}, false
	}
	return proxyBackend{
		name:       "kde",
		set:        kdeSetProxy,
		clearStale: kdeClearStale,
	}, true
}

// kdeWrite wraps kwriteconfig --file kioslaverc. An empty val deletes the
// key (--delete); kwriteconfig rejects a missing value argument outright.
// --notify asks running KIO apps to re-read the config; older kwriteconfig
// builds predate the flag, so a failure retries without it.
func kdeWrite(wcfg, key, group, val string) error {
	base := []string{"--file", "kioslaverc", "--group", group, "--key", key, "--notify"}
	args := append(append([]string{}, base...), "--delete")
	if val != "" {
		args = append(base, val)
	}
	out, err := exec.Command(wcfg, args...).CombinedOutput()
	if err != nil && val != "" {
		// Retry without --notify (old kwriteconfig rejects unknown flags).
		args = []string{"--file", "kioslaverc", "--group", group, "--key", key, val}
		out, err = exec.Command(wcfg, args...).CombinedOutput()
	}
	if err != nil {
		return fmt.Errorf("kwriteconfig %s: %s", key, strings.TrimSpace(string(out)))
	}
	return nil
}

// kdeRead wraps kreadconfig --file kioslaverc; "" when unset.
func kdeRead(rcfg, key, group string) string {
	out, err := exec.Command(rcfg, "--file", "kioslaverc", "--group", group, "--key", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func kdeSetProxy(on bool, addr string) error {
	wcfg := ktool("kwriteconfig")
	if wcfg == "" {
		return fmt.Errorf("kwriteconfig не найден в PATH")
	}
	if !on {
		if err := kdeWrite(wcfg, "mode", "Proxy Settings", "none"); err != nil {
			return err
		}
		return nil
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("адрес прокси %q: %v", addr, err)
	}
	proxyURL := "http://" + net.JoinHostPort(host, port)
	// Mode first: a reader catching the file mid-write sees the proxy URLs
	// already in place before the mode flips to manual.
	for _, kv := range [][2]string{
		{"httpProxy", proxyURL},
		{"httpsProxy", proxyURL},
		{"socksProxy", ""},
		{"mode", "manual"},
	} {
		if err := kdeWrite(wcfg, kv[0], "Proxy Settings", kv[1]); err != nil {
			return err
		}
	}
	return nil
}

// kdeClearStale disables the KDE proxy when its httpProxy points at our own
// HTTP port or at the kill-switch blackhole. Reports whether a stale entry
// was cleared.
func kdeClearStale(localPort int) bool {
	rcfg := ktool("kreadconfig")
	if rcfg == "" {
		return false
	}
	if kdeRead(rcfg, "mode", "Proxy Settings") != "manual" {
		return false
	}
	proxy := strings.TrimPrefix(kdeRead(rcfg, "httpProxy", "Proxy Settings"), "http://")
	host, port, err := net.SplitHostPort(proxy)
	if err != nil {
		return false
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return false
	}
	// Our own footprint: the bridge port or the kill-switch blackhole.
	if port != strconv.Itoa(localPort) && port != sysProxyBlackholePort {
		return false
	}
	wcfg := ktool("kwriteconfig")
	if wcfg == "" {
		return false
	}
	return kdeSetProxy(false, "") == nil
}
