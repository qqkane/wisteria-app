//go:build windows

package core

// Autostart via the HKCU Run key (per-user, no admin).

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const autostartValue = "WisteriaDesktop"

// legacyAutostartValue: the pre-rename entry (v0.3.0 and earlier). It points
// at an exe name that no longer exists, so it is removed opportunistically.
const legacyAutostartValue = "WysteriaDesktop"

func SetAutostart(on bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	_ = k.DeleteValue(legacyAutostartValue)
	if !on {
		return k.DeleteValue(autostartValue)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue(autostartValue, `"`+filepath.Clean(exe)+`"`)
}

func AutostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(autostartValue)
	return err == nil && v != ""
}
