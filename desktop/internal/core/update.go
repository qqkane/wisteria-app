package core

// Update plumbing on top of GitHub releases: version comparison, downloads
// (size-capped), checksum verification and zip extraction. The app (bundling
// the wisteria core) updates from this repo's releases; sing-box and xray
// from their upstream releases.

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	ghTimeout   = 20 * time.Second
	downloadMax = 256 << 20 // single file cap
)

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
	// Digest is the sha256 GitHub computed server-side at upload
	// ("sha256:…"). Upstream cores ship no checksum files at all (sing-box)
	// or per-asset sidecars (xray), but every asset carries this.
	Digest string `json:"digest"`
}

// UpdateInfo is what the UI needs to offer and apply an update.
type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	HasUpdate bool   `json:"has_update"`
	AssetURL  string `json:"asset_url,omitempty"`
	AssetName string `json:"asset_name,omitempty"`
	Digest    string `json:"-"` // sha256 of AssetURL per the GitHub API
}

// ghLatest returns the tag and assets of a repo's latest stable release.
// A missing release page is reported as an error the UI can show as-is.
func ghLatest(repo string) (string, []ghAsset, error) {
	client := &http.Client{Timeout: ghTimeout}
	req, err := http.NewRequest("GET", "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil, errors.New("релизов ещё нет")
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("github: %s", resp.Status)
	}
	var rel struct {
		TagName string    `json:"tag_name"`
		Assets  []ghAsset `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return "", nil, err
	}
	return rel.TagName, rel.Assets, nil
}

// CompareVersions compares dotted numeric versions ("v1.2.3" tolerated).
// Returns -1 when a < b, 0 when equal, 1 when a > b.
func CompareVersions(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		x, y := 0, 0
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(v)), "v")
	var out []int
	for _, p := range strings.Split(v, ".") {
		n := 0
		for _, r := range p {
			if r < '0' || r > '9' {
				break
			}
			n = n*10 + int(r-'0')
		}
		out = append(out, n)
	}
	return out
}

func httpDownload(url, dest string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("скачивание: %s", resp.Status)
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

// verifyDigest checks a downloaded file against the sha256 the GitHub API
// reports for the release asset (computed server-side at upload).
func verifyDigest(file, digest string) error {
	want := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(digest)), "sha256:")
	if len(want) != 64 {
		return errors.New("у ассета релиза нет sha256-digest — обновление отклонено")
	}
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("sha256 не совпал (%s…)", got[:12])
	}
	return nil
}

// verifySHA256 checks file against a "<hash>  <name>" checksum file. When
// the file lists several assets (upstream releases) the line naming our
// file wins; the first valid hash is the fallback (our own single-line
// .sha256 assets).
func verifySHA256(file, checksumFile string) error {
	data, err := os.ReadFile(checksumFile)
	if err != nil {
		return err
	}
	base := filepath.Base(file)
	want, fallback := "", ""
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 1 || len(fields[0]) != 64 {
			continue
		}
		hash := strings.ToLower(fields[0])
		if fallback == "" {
			fallback = hash
		}
		for _, name := range fields[1:] {
			name = strings.TrimPrefix(name, "*") // GNU binary-mode marker
			if name == base {
				want = hash
			}
		}
	}
	if want == "" {
		want = fallback
	}
	if want == "" {
		return errors.New("не удалось прочитать контрольную сумму")
	}
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("sha256 не совпал (%s…)", got[:12])
	}
	return nil
}

// zipExtract copies the entries whose path ends with one of the suffixes
// (flat, by base name) into dir. Base names are sanitized against zip-slip.
func zipExtract(zipPath, dir string, suffixes ...string) ([]string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var out []string
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		name := filepath.ToSlash(zf.Name)
		matched := false
		for _, suf := range suffixes {
			if strings.HasSuffix(name, strings.TrimPrefix(suf, "/")) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		base := filepath.Base(zf.Name)
		if !safeName(base) {
			return out, errors.New("подозрительное имя в архиве: " + zf.Name)
		}
		rc, err := zf.Open()
		if err != nil {
			return out, err
		}
		dest := filepath.Join(dir, base)
		f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			rc.Close()
			return out, err
		}
		n, err := io.Copy(f, rc)
		rc.Close()
		f.Close()
		if err != nil {
			return out, err
		}
		if n > downloadMax {
			return out, errors.New("распакованный файл слишком большой")
		}
		out = append(out, dest)
	}
	if len(out) == 0 {
		return nil, errors.New("в архиве нет нужных файлов")
	}
	return out, nil
}

func safeName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}

// tarExtract copies the entries whose path ends with one of the suffixes
// (flat, by base name) out of a .tar.gz archive into dir. Names are
// sanitized against tar-slip; extracted files are 0755 executables.
func tarExtract(tarPath, dir string, suffixes ...string) ([]string, error) {
	f, err := os.Open(tarPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(io.LimitReader(f, downloadMax+1))
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var out []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return out, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := filepath.ToSlash(hdr.Name)
		matched := false
		for _, suf := range suffixes {
			if strings.HasSuffix(name, strings.TrimPrefix(suf, "/")) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		base := filepath.Base(name)
		if !safeName(base) {
			return out, errors.New("подозрительное имя в архиве: " + hdr.Name)
		}
		dest := filepath.Join(dir, base)
		of, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return out, err
		}
		n, err := io.Copy(of, io.LimitReader(tr, downloadMax+1))
		of.Close()
		if err != nil {
			return out, err
		}
		if n > downloadMax {
			return out, errors.New("распакованный файл слишком большой")
		}
		out = append(out, dest)
	}
	if len(out) == 0 {
		return nil, errors.New("в архиве нет нужных файлов")
	}
	return out, nil
}

// extractArchive dispatches on the archive extension: .zip (windows
// releases, xray upstream) or .tar.gz (linux releases, sing-box upstream).
func extractArchive(archivePath, dir string, suffixes ...string) ([]string, error) {
	if strings.HasSuffix(strings.ToLower(archivePath), ".tar.gz") {
		return tarExtract(archivePath, dir, suffixes...)
	}
	return zipExtract(archivePath, dir, suffixes...)
}

// stageReplace swaps dst for src: the running file is renamed to .old (a
// running exe cannot be deleted or overwritten on Windows, only renamed),
// then src is copied into place. If the copy fails midway the .old file is
// renamed back — without the rollback a failed update left the app (or a
// core) without a binary at all. The .old copy is cleaned up on next start.
func stageReplace(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	old := dst + ".old"
	_ = os.Remove(old)
	movedOld := false
	rollback := func() {
		if movedOld {
			_ = os.Remove(dst)
			_ = os.Rename(old, dst)
		}
	}
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, old); err != nil {
			return fmt.Errorf("переименование %s: %w", dst, err)
		}
		movedOld = true
	}
	in, err := os.Open(src)
	if err != nil {
		rollback()
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		rollback()
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		rollback()
		return err
	}
	if err = out.Close(); err != nil {
		rollback()
		return err
	}
	return nil
}
