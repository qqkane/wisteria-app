//go:build !windows

package core

// Autostart via the XDG autostart convention: a .desktop entry in
// ~/.config/autostart, honored by GNOME, KDE, XFCE and the rest.

import (
	"fmt"
	"os"
	"path/filepath"
)

func autostartFile() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		return "", fmt.Errorf("не удалось определить каталог конфигурации: %v", err)
	}
	return filepath.Join(base, "autostart", "wisteria-desktop.desktop"), nil
}

const desktopEntry = `[Desktop Entry]
Type=Application
Name=Wisteria Desktop
Exec="%s"
Terminal=false
X-GNOME-UsesNotifications=false
Categories=Network;
`

func SetAutostart(on bool) error {
	path, err := autostartFile()
	if err != nil {
		return err
	}
	if !on {
		return os.Remove(path)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(fmt.Sprintf(desktopEntry, filepath.Clean(exe))), 0o644)
}

func AutostartEnabled() bool {
	path, err := autostartFile()
	if err != nil {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
