package core

// App self-update: the release bundle produced by CI, checksummed by a
// sibling .sha256 asset. Which bundle the app picks depends on how it is
// installed: a zip/tar.gz unpacked on a writable disk (stage the binary in
// place) or an AppImage (stage the image file itself — the mount the binary
// reports is squashfs and read-only). The user restarts from the UI
// afterwards.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const repoApp = "qqkane/wisteria-app"

// AppImagePath returns the .AppImage file this process was launched from, or
// "" when the app runs unpacked. The AppImage runtime exports APPIMAGE with
// the absolute path; inside the image os.Executable() reports
// /tmp/.mount_*/usr/bin/WisteriaDesktop, which lives on a read-only squashfs
// mount — every write there fails with EROFS, so the file to replace is the
// image itself, not the path the binary reports. Renamed images are fine:
// the runtime still points APPIMAGE at the real file.
func AppImagePath() string {
	p := strings.TrimSpace(os.Getenv("APPIMAGE"))
	if p == "" {
		return ""
	}
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		return ""
	}
	return p
}

// Release asset and binary naming, per platform and install layout: CI
// publishes a zip for windows, a tar.gz and an AppImage for linux, and the
// updater picks the one it can actually write to.
//
// appAssetFor is a pure function of (goos, appImagePath) so the choice is
// testable without an AppImage mount.
func appAssetFor(goos, appImagePath string) string {
	switch {
	case goos == "windows":
		return "WisteriaDesktop-windows-amd64.zip"
	case goos == "linux" && appImagePath != "":
		return "WisteriaDesktop-linux-x86_64.AppImage"
	}
	return "WisteriaDesktop-linux-amd64.tar.gz"
}

func appAssetName() string { return appAssetFor(runtime.GOOS, AppImagePath()) }

func appBinName() string {
	if runtime.GOOS == "windows" {
		return "WisteriaDesktop.exe"
	}
	return "WisteriaDesktop"
}

func coreBinName() string { return "wisteria-client" + exeSuffix }

// AppUpdateCheck compares appVersion with the latest published release.
func AppUpdateCheck(current string) (UpdateInfo, error) {
	info := UpdateInfo{Current: current}
	tag, assets, err := ghLatest(repoApp)
	if err != nil {
		return info, err
	}
	assetName := appAssetName()
	info.Latest = strings.TrimPrefix(tag, "v")
	var zipAsset, shaAsset ghAsset
	for _, a := range assets {
		switch a.Name {
		case assetName:
			zipAsset = a
		case assetName + ".sha256":
			shaAsset = a
		}
	}
	if zipAsset.Name == "" {
		return info, fmt.Errorf("в релизе нет %s", assetName)
	}
	if shaAsset.Name == "" {
		return info, errors.New("в релизе нет файла контрольной суммы")
	}
	info.AssetURL, info.AssetName = zipAsset.URL, assetName
	info.zipID, info.shaID = zipAsset.ID, shaAsset.ID
	info.HasUpdate = CompareVersions(current, info.Latest) < 0
	return info, nil
}

// AppUpdateApply re-checks, downloads the release bundle, verifies the sha256
// and stages the new app in place of the running one (plus the bundled
// wisteria core, when the bundle carries it). Returns the new version; the
// caller restarts to switch to it.
func AppUpdateApply(current string) (string, error) {
	info, err := AppUpdateCheck(current)
	if err != nil {
		return "", err
	}
	if !info.HasUpdate {
		return "", errors.New("установлена актуальная версия")
	}
	tmp, err := os.MkdirTemp("", "wisteria-update")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	zipPath := filepath.Join(tmp, info.AssetName)
	if err := ghDownload(repoApp, info.zipID, zipPath); err != nil {
		return "", err
	}
	// The checksum asset lives next to the bundle (<asset>.sha256).
	shaPath := zipPath + ".sha256"
	if err := ghDownload(repoApp, info.shaID, shaPath); err != nil {
		return "", err
	}
	if err := verifySHA256(zipPath, shaPath); err != nil {
		return "", err
	}
	// An AppImage is not an archive: the image file itself is the new app,
	// and it already carries the new core inside.
	if img := AppImagePath(); img != "" && strings.HasSuffix(info.AssetName, ".AppImage") {
		if err := stageAppBinary(zipPath, img, info.AssetName); err != nil {
			return "", err
		}
		return info.Latest, nil
	}
	staged, err := extractArchive(zipPath, tmp, appBinName(), coreBinName())
	if err != nil {
		return "", err
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	for _, src := range staged {
		switch base := filepath.Base(src); base {
		case appBinName():
			if err := stageAppBinary(src, exe, info.AssetName); err != nil {
				return "", err
			}
		case coreBinName():
			if bin := BinDir(); bin != "" {
				// Best effort: the core may be locked in exotic setups; the
				// app exe is the important one.
				_ = stageReplace(src, filepath.Join(bin, base))
			}
		}
	}
	return info.Latest, nil
}

// stageAppBinary puts the freshly verified app binary (or image) in place of
// dst, but refuses first when dst's directory takes no writes: an AppImage
// mount, a system-wide install dir or a read-only volume all used to fail
// deep inside stageReplace with "переименование …: read-only file system",
// which tells the user nothing they could act on.
func stageAppBinary(src, dst, assetName string) error {
	dir := filepath.Dir(dst)
	if !dirWritable(dir) {
		return fmt.Errorf("каталог %s доступен только на чтение — отсюда приложение себя обновить не может. Скачайте %s со страницы релиза и замените файл вручную", dir, assetName)
	}
	return stageReplace(src, dst)
}

// CleanupOldUpdate removes the .old copies staged by a previous update.
func CleanupOldUpdate() {
	exe, err := os.Executable()
	if err == nil {
		_ = os.Remove(exe + ".old")
	}
	// An AppImage update renames the running image aside; its mount keeps
	// working off that inode until the app restarts, so this is the first
	// moment the leftover is safe to delete.
	if img := AppImagePath(); img != "" {
		_ = os.Remove(img + ".old")
	}
	if bin := BinDir(); bin != "" {
		entries, _ := os.ReadDir(bin)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".old") {
				_ = os.Remove(filepath.Join(bin, e.Name()))
			}
		}
	}
}
