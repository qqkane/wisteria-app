//go:build !windows

package core

// System proxy dispatch for Linux. Two backends: GNOME/gsettings (also
// respected by most GTK desktops) and KDE/kioslaverc (written via
// kwriteconfig, read by KIO). The backend is picked from the running
// desktop; when nothing recognizable runs, whichever tool is installed
// gets the vote (kwriteconfig first — a stale gsettings on a headless box
// is the more confusing failure).

import (
	"fmt"
	"os"
	"strings"
)

// desktopIsKDE reports whether the session looks like KDE Plasma.
func desktopIsKDE() bool {
	if os.Getenv("KDE_FULL_SESSION") != "" {
		return true
	}
	return strings.Contains(strings.ToUpper(os.Getenv("XDG_CURRENT_DESKTOP")), "KDE")
}

// SetSystemProxy enables (addr = "host:port") or disables the desktop
// system proxy. See sysproxy_gnome.go / sysproxy_kde.go for the backends.
func SetSystemProxy(on bool, addr string) error {
	b, err := pickProxyBackend()
	if err != nil {
		return err
	}
	return b.set(on, addr)
}

// ClearStaleSystemProxy disables the system proxy when it points at our own
// HTTP port — the mark of a previous run that died without teardown. Every
// installed backend is consulted (a previous run may have used another
// desktop).
func ClearStaleSystemProxy(localPort int) bool {
	cleared := false
	for _, b := range availableProxyBackends() {
		if b.clearStale(localPort) {
			cleared = true
		}
	}
	return cleared
}

type proxyBackend struct {
	name       string
	set        func(on bool, addr string) error
	clearStale func(localPort int) bool
}

// pickProxyBackend returns the backend for the running desktop, falling
// back to any installed one; an error names what the user can install.
func pickProxyBackend() (proxyBackend, error) {
	if desktopIsKDE() {
		if b, ok := kdeBackend(); ok {
			return b, nil
		}
		if g, ok := gnomeBackend(); ok {
			return g, nil
		}
		return proxyBackend{}, fmt.Errorf("системный прокси: KDE-сессия обнаружена, но kwriteconfig5/kwriteconfig6 не найден в PATH")
	}
	if b, ok := gnomeBackend(); ok {
		return b, nil
	}
	if b, ok := kdeBackend(); ok {
		return b, nil
	}
	return proxyBackend{}, fmt.Errorf("системный прокси на этой системе не поддерживается (нужен GNOME/gsettings или KDE/kwriteconfig)")
}

// availableProxyBackends lists every backend whose tool is installed,
// preferring the one matching the running desktop.
func availableProxyBackends() []proxyBackend {
	var out []proxyBackend
	if b, ok := preferredBackend(); ok {
		out = append(out, b)
	}
	if b, ok := gnomeBackend(); ok && !outHas(out, b.name) {
		out = append(out, b)
	}
	if b, ok := kdeBackend(); ok && !outHas(out, b.name) {
		out = append(out, b)
	}
	return out
}

func preferredBackend() (proxyBackend, bool) {
	if desktopIsKDE() {
		return kdeBackend()
	}
	return gnomeBackend()
}

func outHas(bs []proxyBackend, name string) bool {
	for _, b := range bs {
		if b.name == name {
			return true
		}
	}
	return false
}
