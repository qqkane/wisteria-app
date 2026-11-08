//go:build !windows

package core

// GNOME system proxy backend: gsettings (the de-facto system-wide proxy
// settings on GNOME and GTK-based desktops; read by libgsettings consumers).
// Best effort by design: on a desktop without gsettings the backend simply
// reports unavailable.

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
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("gsettings %s: %s", strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

func gsettingsAvailable() bool {
	_, err := exec.LookPath("gsettings")
	return err == nil
}

func gnomeBackend() (proxyBackend, bool) {
	if !gsettingsAvailable() {
		return proxyBackend{}, false
	}
	return proxyBackend{
		name:       "gnome",
		set:        gnomeSetProxy,
		clearStale: gnomeClearStale,
	}, true
}

func gnomeSetProxy(on bool, addr string) error {
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
	// Only mode/host/port are set: the http/https enable-proxy and
	// use-same-proxy keys were removed from the schema in GLib 2.74+, and
	// every post-2.74 gsettings fails the whole call with "No such key".
	ignore := "['localhost', '127.0.0.0/8', '::1', '10.0.0.0/8', '172.16.0.0/12', '192.168.0.0/16']"
	for _, scheme := range []struct{ schema string }{
		{"org.gnome.system.proxy.http"},
		{"org.gnome.system.proxy.https"},
	} {
		if _, err := gsettings("set", scheme.schema, "host", host); err != nil {
			return err
		}
		if _, err := gsettings("set", scheme.schema, "port", port); err != nil {
			return err
		}
	}
	if _, err := gsettings("set", "org.gnome.system.proxy", "ignore-hosts", ignore); err != nil {
		return err
	}
	return nil
}

func SystemProxySupported() bool { return len(availableProxyBackends()) > 0 }

// gnomeClearStale disables the GNOME proxy when it points at our own HTTP
// port or at the kill-switch blackhole. Reports whether a stale entry was
// cleared.
func gnomeClearStale(localPort int) bool {
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
	// Our own footprint: the bridge port or the kill-switch blackhole.
	if p := strings.TrimSpace(port); p != strconv.Itoa(localPort) && p != sysProxyBlackholePort {
		return false
	}
	return gnomeSetProxy(false, "") == nil
}
