package core

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestWintunPathScanOrder(t *testing.T) {
	sb := t.TempDir()
	dirs := Dirs{Runtime: t.TempDir(), Data: t.TempDir()}
	if got := WintunPath(filepath.Join(sb, "sing-box.exe"), dirs); got != "" {
		t.Fatalf("no dll anywhere: got %q", got)
	}

	// The data dir is the last candidate the TUN gate checks.
	dll := filepath.Join(dirs.Data, "wintun.dll")
	if err := os.WriteFile(dll, []byte("dll"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := WintunPath(filepath.Join(sb, "sing-box.exe"), dirs); got != dll {
		t.Fatalf("dll in data dir: got %q, want %q", got, dll)
	}
	if !WintunInstalled(filepath.Join(sb, "sing-box.exe"), dirs) {
		t.Fatal("WintunInstalled must mirror WintunPath")
	}

	// The sing-box binary's own directory wins over the app dirs — an
	// explicitly configured external sing-box finds the dll beside it.
	own := filepath.Join(sb, "wintun.dll")
	if err := os.WriteFile(own, []byte("dll"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := WintunPath(filepath.Join(sb, "sing-box.exe"), dirs); got != own {
		t.Fatalf("dll next to sing-box: got %q, want %q", got, own)
	}

	// An empty sing-box path must not probe the working directory.
	if got := WintunPath("", dirs); got != dll {
		t.Fatalf("empty singbox: got %q, want %q", got, dll)
	}
}

func TestWintunArchiveFilter(t *testing.T) {
	// The official wintun.net layout: four architecture copies under
	// wintun/bin/<arch>/ plus the license. The path-qualified filters must
	// stage exactly the amd64 DLL and the license.
	dir := t.TempDir()
	zp := filepath.Join(dir, "wintun-0.14.1.zip")
	f, err := os.Create(zp)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	files := []string{
		"wintun/LICENSE.txt",
		"wintun/README.md",
		"wintun/bin/amd64/wintun.dll",
		"wintun/bin/arm64/wintun.dll",
		"wintun/bin/arm/wintun.dll",
		"wintun/bin/x86/wintun.dll",
		"wintun/include/wintun.h",
	}
	for _, name := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	out := t.TempDir()
	staged, err := extractArchive(zp, out, "/bin/amd64/wintun.dll", "/LICENSE.txt")
	if err != nil {
		t.Fatalf("extractArchive: %v", err)
	}
	var haveDll, haveLic, extra bool
	for _, p := range staged {
		switch filepath.Base(p) {
		case "wintun.dll":
			if haveDll || filepath.Dir(p) != out {
				extra = true // more than one arch copy staged
			}
			haveDll = true
		case "LICENSE.txt":
			haveLic = true
		default:
			extra = true
		}
	}
	if !haveDll || !haveLic || extra {
		t.Fatalf("staged = %v, want exactly one wintun.dll + LICENSE.txt", staged)
	}
}

func TestWintunDestDirsOrder(t *testing.T) {
	sb := t.TempDir()
	dirs := Dirs{Data: t.TempDir()}
	got := wintunDestDirs(filepath.Join(sb, "sing-box.exe"), dirs)
	if len(got) < 3 {
		t.Fatalf("candidates = %v, want the sing-box dir, bin/ and data dir", got)
	}
	if got[0] != sb {
		t.Fatalf("first candidate = %q, want the sing-box dir %q", got[0], sb)
	}
	seen := map[string]bool{}
	for _, d := range got {
		if seen[d] {
			t.Fatalf("duplicate candidate %q in %v", d, got)
		}
		seen[d] = true
	}
}

// TestWintunDestDirsRuntimeAndQuotedPath: the installer must scan the same
// directories WintunPath reads (the runtime dir used to be skipped), and a
// quoted "Copy as path" value must be cleaned before taking its dir.
func TestWintunDestDirsRuntimeAndQuotedPath(t *testing.T) {
	sb := t.TempDir()
	runtimeDir := t.TempDir()
	dirs := Dirs{Runtime: runtimeDir, Data: t.TempDir()}
	quoted := `"` + filepath.Join(sb, "sing-box.exe") + `"`
	got := wintunDestDirs(quoted, dirs)
	if len(got) == 0 || got[0] != sb {
		t.Fatalf("candidates = %v, want the sing-box dir %q first", got, sb)
	}
	hasRuntime := false
	for _, d := range got {
		if d == runtimeDir {
			hasRuntime = true
		}
	}
	if !hasRuntime {
		t.Fatalf("candidates = %v, want the runtime dir %q", got, runtimeDir)
	}
}

func TestWintunPathQuotedSingbox(t *testing.T) {
	sb := t.TempDir()
	dll := filepath.Join(sb, "wintun.dll")
	if err := os.WriteFile(dll, []byte("dll"), 0o755); err != nil {
		t.Fatal(err)
	}
	quoted := `"` + filepath.Join(sb, "sing-box.exe") + `"`
	if got := WintunPath(quoted, Dirs{}); got != dll {
		t.Fatalf("WintunPath(quoted) = %q, want %q", got, dll)
	}
}
