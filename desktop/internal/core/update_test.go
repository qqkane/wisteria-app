package core

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.6.8", "0.6.9", -1},
		{"v0.6.9", "0.6.9", 0},
		{"0.10.0", "0.9.5", 1},
		{"1.12.14", "1.12.14", 0},
		{"1.12", "1.12.0", 0},     // missing parts pad with zero
		{"25.8.29", "1.99.99", 1}, // date-style xray versions compare numerically
		{"0.6.8", "0.6.10", -1},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestProbeVersionParsing(t *testing.T) {
	// The regex powering probeVersion, against real output shapes.
	for out, want := range map[string]string{
		"sing-box version 1.12.14\n\nEnvironment: go1.24": "1.12.14",
		"Xray 25.8.29 (Xray, Penetrates All.)":            "25.8.29",
		"Wisteria client v0.7.1 (abc123)":                 "0.7.1",
		"no version here":                                 "",
	} {
		if got := versionToken.FindString(out); got != want && trimV(got) != want {
			t.Errorf("token(%q) = %q, want %q", out, got, want)
		}
	}
}

func trimV(s string) string {
	if len(s) > 0 && s[0] == 'v' {
		return s[1:]
	}
	return s
}

func TestPickCoreAsset(t *testing.T) {
	assets := []ghAsset{
		{Name: "sing-box-1.12.14-linux-amd64.tar.gz", URL: "u3"},
		{Name: "sing-box-1.12.14-windows-arm64.zip"},
		{Name: "sing-box-1.12.14-windows-amd64.zip", URL: "u1"},
		{Name: "SFA-1.12.14.apk"},
	}
	wantSing := "u1"
	if runtime.GOOS != "windows" {
		wantSing = "u3"
	}
	a, err := pickCoreAsset("sing-box", assets)
	if err != nil || a.URL != wantSing {
		t.Fatalf("sing-box asset = %v, %v (want %s)", a, err, wantSing)
	}
	xr := []ghAsset{
		{Name: "Xray-linux-64.zip", URL: "u4"},
		{Name: "Xray-windows-64.zip", URL: "u2"},
		{Name: "Xray-windows-32.zip"},
	}
	wantXr := "u2"
	if runtime.GOOS != "windows" {
		wantXr = "u4"
	}
	a, err = pickCoreAsset("xray", xr)
	if err != nil || a.URL != wantXr {
		t.Fatalf("xray asset = %v, %v (want %s)", a, err, wantXr)
	}
	if _, err := pickCoreAsset("xray", assets[:0]); err == nil {
		t.Fatal("empty asset list must error")
	}
}

// writeTar builds a real .tar.gz fixture with the given entries.
func writeTar(t *testing.T, path string, entries ...[2]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	for _, e := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: e[0], Mode: 0o755, Size: int64(len(e[1])), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e[1])); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTarExtract(t *testing.T) {
	dir := t.TempDir()
	arc := filepath.Join(dir, "sb.tar.gz")
	writeTar(t, arc,
		[2]string{"sing-box-1.12.14/sing-box", "BINARY"},
		[2]string{"sing-box-1.12.14/README.md", "readme"},
	)
	out, err := tarExtract(arc, dir, "/sing-box")
	if err != nil || len(out) != 1 || filepath.Base(out[0]) != "sing-box" {
		t.Fatalf("tarExtract = %v, %v", out, err)
	}
	b, err := os.ReadFile(out[0])
	if err != nil || string(b) != "BINARY" {
		t.Fatalf("content = %q, %v", b, err)
	}

	// A "../evil" entry must not escape the extraction dir: like
	// zipExtract, the entry is written flat by its base name.
	bad := filepath.Join(dir, "bad.tar.gz")
	writeTar(t, bad, [2]string{"../evil", "x"}, [2]string{"deep/dir/evil", "y"})
	out2, err := tarExtract(bad, dir, "/evil")
	if err != nil || len(out2) != 2 {
		t.Fatalf("tarExtract slip = %v, %v", out2, err)
	}
	for _, got := range out2 {
		if filepath.Dir(got) != dir {
			t.Fatalf("entry escaped the dir: %s", got)
		}
	}
}
