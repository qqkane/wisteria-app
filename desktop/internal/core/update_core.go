package core

// Third-party core updates (sing-box, xray) from their upstream GitHub
// releases. Only the copy in the app's own bin/ is replaced: an explicitly
// configured external path (e.g. pointing into v2rayN's directory) is never
// touched.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const (
	repoSingBox = "SagerNet/sing-box"
	repoXray    = "XTLS/Xray-core"
)

// BinDir is the app's own core directory (<exe dir>/bin), "" if unknown.
func BinDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "bin")
}

var versionToken = regexp.MustCompile(`v?\d+(?:\.\d+)+`)

// probeVersion runs `<core> version` and extracts the first version-looking
// token ("sing-box version 1.12.14", "Xray 25.8.29 ...").
func probeVersion(bin string) string {
	if bin == "" {
		return ""
	}
	out, err := runCoreOutput(bin, "version")
	if err != nil && out == "" {
		return ""
	}
	if m := versionToken.FindString(out); m != "" {
		return strings.TrimPrefix(m, "v")
	}
	return ""
}

// pickCoreAsset chooses the amd64 archive of a core release for the
// running platform (sing-box ships tar.gz for linux, zips for windows).
func pickCoreAsset(name string, assets []ghAsset) (ghAsset, error) {
	for _, a := range assets {
		l := strings.ToLower(a.Name)
		switch name {
		case "sing-box":
			if runtime.GOOS == "windows" {
				if strings.HasSuffix(l, ".zip") && strings.Contains(l, "windows-amd64") && !strings.Contains(l, "android") {
					return a, nil
				}
			} else {
				if strings.HasSuffix(l, ".tar.gz") && strings.Contains(l, "linux-amd64") && !strings.Contains(l, "android") {
					return a, nil
				}
			}
		case "xray":
			if !strings.HasSuffix(l, ".zip") {
				continue
			}
			if runtime.GOOS == "windows" && l == "xray-windows-64.zip" {
				return a, nil
			}
			if runtime.GOOS == "linux" && l == "xray-linux-64.zip" {
				return a, nil
			}
		}
	}
	return ghAsset{}, fmt.Errorf("в релизе нет архива для %s amd64", runtime.GOOS)
}

// CoreUpdateCheck looks up the latest upstream release for "sing-box" or
// "xray". The installed version (if the binary is resolvable) is probed to
// decide HasUpdate; an unprobeable binary leaves the choice to the user.
func CoreUpdateCheck(name, explicitPath string) (UpdateInfo, error) {
	info := UpdateInfo{}
	repo := ""
	switch name {
	case "sing-box":
		repo = repoSingBox
	case "xray":
		repo = repoXray
	default:
		return info, errors.New("неизвестное ядро")
	}
	tag, assets, err := ghLatest(repo)
	if err != nil {
		return info, err
	}
	info.Latest = strings.TrimPrefix(tag, "v")
	asset, err := pickCoreAsset(name, assets)
	if err != nil {
		return info, err
	}
	info.AssetURL, info.AssetName = asset.URL, asset.Name
	info.HasUpdate = true
	if path := corePathFor(name, explicitPath); path != "" {
		info.Current = probeVersion(path)
	}
	if info.Current != "" {
		info.HasUpdate = CompareVersions(info.Current, info.Latest) < 0
	}
	return info, nil
}

// corePathFor resolves the binary whose version Check reports: the explicit
// path if set, else the app's own bin/ copy, else whatever FindCore finds.
func corePathFor(name, explicitPath string) string {
	if explicitPath != "" {
		return explicitPath
	}
	exe := name + exeSuffix
	if bin := BinDir(); bin != "" {
		p := filepath.Join(bin, exe)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return FindCore(exe)
}

// CoreUpdateApply downloads the latest release and replaces the app's own
// bin/<name>.exe via the rename trick. The service must not be running.
func CoreUpdateApply(name, explicitPath string) (string, error) {
	if explicitPath != "" {
		return "", errors.New("ядро подключено по внешнему пути — обновите его вне приложения")
	}
	bin := BinDir()
	if bin == "" {
		return "", errors.New("не удалось определить каталог приложения")
	}
	info, err := CoreUpdateCheck(name, explicitPath)
	if err != nil {
		return "", err
	}
	if !info.HasUpdate {
		return "", fmt.Errorf("%s актуален (v%s)", name, info.Current)
	}
	tmp, err := os.MkdirTemp("", "wisteria-core")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	zipPath := filepath.Join(tmp, info.AssetName)
	if err := httpDownload(info.AssetURL, zipPath); err != nil {
		return "", err
	}
	staged, err := extractArchive(zipPath, tmp, "/"+name+exeSuffix)
	if err != nil {
		return "", err
	}
	if err := stageReplace(staged[0], filepath.Join(bin, name+exeSuffix)); err != nil {
		return "", err
	}
	return info.Latest, nil
}
