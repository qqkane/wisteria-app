//go:build !windows

package core

// Autostart and desktop/application menu shortcuts via the XDG Desktop Entry
// specification: ~/.config/autostart, ~/.local/share/applications, and ~/Desktop.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var embeddedIconPNG []byte

// SetAppIcon stores the PNG icon bytes embedded in the desktop binary so Linux
// XDG shortcuts can install it into ~/.local/share/icons/hicolor/256x256/apps/.
func SetAppIcon(pngData []byte) {
	embeddedIconPNG = pngData
}

func resolveDesktopExec() (string, error) {
	// An AppImage's os.Executable() is the ephemeral squashfs mount point
	// (/tmp/.mount_*): it vanishes when the app exits, so a shortcut built
	// from it points at a dead path. The image file itself is the stable target.
	exe := AppImagePath()
	if exe == "" {
		var err error
		exe, err = os.Executable()
		if err != nil {
			return "", err
		}
	}
	return filepath.Clean(exe), nil
}

func formatExecPath(exe string) string {
	if strings.ContainsAny(exe, " \t\"'\\$`") {
		escaped := strings.ReplaceAll(exe, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return exe
}

func xdgDataHome() string {
	if v := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "share")
}

func ensureLinuxIcon() {
	if len(embeddedIconPNG) == 0 {
		return
	}
	dataHome := xdgDataHome()
	if dataHome == "" {
		return
	}
	iconDir := filepath.Join(dataHome, "icons", "hicolor", "256x256", "apps")
	if err := os.MkdirAll(iconDir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(iconDir, "wisteria-desktop.png"), embeddedIconPNG, 0o644)
}

func buildDesktopEntry(exe string, autostart bool) string {
	var b strings.Builder
	b.WriteString("[Desktop Entry]\n")
	b.WriteString("Type=Application\n")
	b.WriteString("Version=1.0\n")
	b.WriteString("Name=Wisteria Desktop\n")
	b.WriteString("GenericName=VPN Client\n")
	b.WriteString("Comment=Anti-DPI VPN and proxy client\n")
	fmt.Fprintf(&b, "Exec=%s\n", formatExecPath(exe))
	b.WriteString("Icon=wisteria-desktop\n")
	b.WriteString("Terminal=false\n")
	b.WriteString("Categories=Network;VPN;\n")
	b.WriteString("StartupWMClass=Wisteria Desktop\n")
	if autostart {
		b.WriteString("X-GNOME-Autostart-enabled=true\n")
		b.WriteString("X-KDE-autostart-after=panel\n")
	}
	return b.String()
}

func autostartFile() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		return "", fmt.Errorf("не удалось определить каталог конфигурации: %v", err)
	}
	return filepath.Join(base, "autostart", "wisteria-desktop.desktop"), nil
}

func SetAutostart(on bool) error {
	path, err := autostartFile()
	if err != nil {
		return err
	}
	if !on {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe, err := resolveDesktopExec()
	if err != nil {
		return err
	}
	ensureLinuxIcon()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(buildDesktopEntry(exe, true)), 0o644)
}

func AutostartEnabled() bool {
	path, err := autostartFile()
	if err != nil {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func userDesktopDir() string {
	if out, err := exec.Command("xdg-user-dir", "DESKTOP").Output(); err == nil {
		if p := strings.TrimSpace(string(out)); p != "" {
			if st, serr := os.Stat(p); serr == nil && st.IsDir() {
				return p
			}
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	for _, cand := range []string{"Desktop", "Рабочий стол"} {
		p := filepath.Join(home, cand)
		if st, serr := os.Stat(p); serr == nil && st.IsDir() {
			return p
		}
	}
	return ""
}

// InstallLinuxShortcuts creates the application menu entry (~/.local/share/applications)
// and, when includeDesktop is true, a trusted launcher on the user's Desktop.
func InstallLinuxShortcuts(includeDesktop bool) (string, error) {
	exe, err := resolveDesktopExec()
	if err != nil {
		return "", err
	}
	ensureLinuxIcon()
	dataHome := xdgDataHome()
	if dataHome == "" {
		return "", fmt.Errorf("не удалось определить XDG_DATA_HOME")
	}
	appsDir := filepath.Join(dataHome, "applications")
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		return "", err
	}
	content := []byte(buildDesktopEntry(exe, false))
	appEntry := filepath.Join(appsDir, "wisteria-desktop.desktop")
	if err := os.WriteFile(appEntry, content, 0o644); err != nil {
		return "", err
	}
	if _, err := exec.LookPath("update-desktop-database"); err == nil {
		_ = exec.Command("update-desktop-database", appsDir).Run()
	}
	if includeDesktop {
		deskDir := userDesktopDir()
		if deskDir == "" {
			return appEntry, fmt.Errorf("ярлык в меню создан, но каталог рабочего стола не найден")
		}
		deskEntry := filepath.Join(deskDir, "wisteria-desktop.desktop")
		// 0755 is required by GNOME/KDE/XFCE to mark a Desktop .desktop file as trusted.
		if err := os.WriteFile(deskEntry, content, 0o755); err != nil {
			return appEntry, err
		}
		if _, err := exec.LookPath("gio"); err == nil {
			_ = exec.Command("gio", "set", deskEntry, "metadata::trusted", "true").Run()
		}
		return deskEntry, nil
	}
	return appEntry, nil
}
