package core

// App self-update: the release zip produced by CI (WisteriaDesktop.exe +
// bin/wisteria-client.exe + README), checksummed by a sibling .sha256 asset.
// The running exe is staged via the rename trick; the user restarts from the
// UI afterwards.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const repoApp = "qqkane/wisteria-app"

// Release asset and binary naming, per platform: CI publishes a zip bundle
// for windows and a tar.gz bundle for linux, the updater picks its own.
func appAssetName() string {
	if runtime.GOOS == "windows" {
		return "WisteriaDesktop-windows-amd64.zip"
	}
	return "WisteriaDesktop-linux-amd64.tar.gz"
}

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

// AppUpdateApply re-checks, downloads the release zip, verifies the sha256
// and stages WisteriaDesktop.exe (and the bundled wisteria core, when the
// zip carries it) next to the running files. Returns the new version; the
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
			if err := stageReplace(src, exe); err != nil {
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

// CleanupOldUpdate removes the .old copies staged by a previous update.
func CleanupOldUpdate() {
	exe, err := os.Executable()
	if err == nil {
		_ = os.Remove(exe + ".old")
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
