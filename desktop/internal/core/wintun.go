package core

// wintun.dll management for Windows TUN mode. sing-box (sing-tun) loads
// wintun.dll — the official WireGuard TUN API library with the signed
// driver embedded inside — from disk when the tun inbound starts. Upstream
// sing-box releases stopped bundling the DLL, so the app installs it
// itself: from the pinned official wintun.net archive, verified by the
// published sha256, next to the resolved sing-box binary (the first place
// hasWintunDLL looks). The prebuilt-binaries license that ships in the
// archive permits redistributing the unmodified DLL with software that
// uses the wintun.h API — which sing-box does — as long as the license
// file travels with it.

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	// The only version currently published; the DLL embeds the driver and
	// works on every Windows release the app supports.
	wintunVersion = "0.14.1"
	// sha256 published on wintun.net for the archive below.
	wintunZipSHA = "07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51"
	wintunZipURL = "https://www.wintun.net/builds/wintun-0.14.1.zip"
)

// WintunPath returns the wintun.dll sing-box would load for the given
// sing-box binary — its own directory first, then the app dirs — or ""
// when none exists. hasWintunDLL (the startTun gate) is the boolean form.
func WintunPath(singbox string, dirs Dirs) string {
	for _, d := range wintunLookupDirs(singbox, dirs) {
		p := filepath.Join(d, "wintun.dll")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// wintunLookupDirs is the single source of the search/install order:
// next to the resolved sing-box binary, the app's own bin/, the runtime
// dir, then the data dir. Installation and the startTun gate used to
// disagree (the installer skipped the runtime dir, the gate skipped bin/),
// so a DLL could be reported installed and still not be found.
func wintunLookupDirs(singbox string, dirs Dirs) []string {
	var out []string
	seen := map[string]bool{}
	add := func(d string) {
		if d == "" {
			return
		}
		if d = filepath.Clean(d); !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	if sb := cleanCorePath(singbox); sb != "" {
		add(filepath.Dir(sb))
	}
	add(BinDir())
	add(dirs.Runtime)
	add(dirs.Data)
	return out
}

// WintunInstalled reports whether TUN's DLL requirement is satisfied.
func WintunInstalled(singbox string, dirs Dirs) bool {
	return WintunPath(singbox, dirs) != ""
}

// RuntimeOS reports the build's operating system to the UI layer, so the
// settings screen can show the wintun row on Windows only.
func RuntimeOS() string { return runtime.GOOS }

// WintunApply downloads the pinned official wintun archive (sha256-verified),
// extracts the amd64 DLL plus its license and installs both into the first
// writable candidate directory: next to the resolved sing-box binary when
// that directory accepts writes (covers explicitly configured external
// paths), else the app's own bin/, else the data dir hasWintunDLL also
// scans. Returns the directory the DLL landed in.
func WintunApply(singbox string, dirs Dirs) (string, error) {
	if runtime.GOOS != "windows" {
		return "", errors.New("wintun.dll нужен только на Windows")
	}
	tmp, err := os.MkdirTemp("", "wisteria-wintun")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	zipPath := filepath.Join(tmp, "wintun.zip")
	if err := httpDownload(wintunZipURL, zipPath); err != nil {
		return "", err
	}
	if err := verifyDigest(zipPath, "sha256:"+wintunZipSHA); err != nil {
		return "", fmt.Errorf("wintun %s: %v", wintunVersion, err)
	}
	// The archive nests everything under wintun/ and carries four
	// architecture copies; the path-qualified filters pick exactly the
	// amd64 DLL (base-name matching alone would extract all four) and the
	// prebuilt-binaries license.
	staged, err := extractArchive(zipPath, tmp, "/bin/amd64/wintun.dll", "/LICENSE.txt")
	if err != nil {
		return "", err
	}
	var dll, lic string
	for _, f := range staged {
		switch filepath.Base(f) {
		case "wintun.dll":
			dll = f
		case "LICENSE.txt":
			lic = f
		}
	}
	if dll == "" {
		return "", errors.New("в архиве wintun нет amd64 wintun.dll")
	}
	var lastErr error
	for _, dst := range wintunDestDirs(singbox, dirs) {
		if err := stageReplace(dll, filepath.Join(dst, "wintun.dll")); err != nil {
			lastErr = err
			continue
		}
		if lic != "" {
			// The redistribution terms require the license to travel with
			// the DLL. Best effort: its absence is a paperwork gap, not a
			// broken TUN.
			if data, err := os.ReadFile(lic); err == nil && len(data) > 0 {
				_ = os.WriteFile(filepath.Join(dst, "wintun-LICENSE.txt"), data, 0o644)
			}
		}
		return dst, nil
	}
	if lastErr == nil {
		lastErr = errors.New("не найден каталог для установки")
	}
	return "", fmt.Errorf("wintun.dll: %v", lastErr)
}

// wintunDestDirs lists the install candidates in WintunPath's lookup
// order, de-duplicated. It is the same list by construction: a DLL that
// installs into a directory the gate does not scan is indistinguishable
// from a missing DLL.
func wintunDestDirs(singbox string, dirs Dirs) []string {
	return wintunLookupDirs(singbox, dirs)
}

// httpDownload fetches a plain https URL to dest with the same size cap
// and error surface as ghDownload (which is tied to the GitHub asset API).
func httpDownload(url, dest string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("загрузка: %s", resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := io.Copy(f, io.LimitReader(resp.Body, downloadMax+1))
	if err != nil {
		return err
	}
	if n > downloadMax {
		return errors.New("файл слишком большой")
	}
	return nil
}
