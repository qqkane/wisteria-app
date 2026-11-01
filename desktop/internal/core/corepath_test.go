package core

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCleanCorePath(t *testing.T) {
	cases := [][2]string{
		{`"C:\Tools\wisteria-client.exe"`, `C:\Tools\wisteria-client.exe`}, // Windows "Copy as path"
		{` 'C:\Tools\xray.exe' `, `C:\Tools\xray.exe`},                     // single quotes + padding
		{`  /opt/wisteria/bin/xray  `, `/opt/wisteria/bin/xray`},
		{`bin/wisteria-client.exe`, `bin/wisteria-client.exe`}, // relative stays relative
		{`"`, `"`}, // a lone quote is not a wrapper
		{`"unclosed`, `"unclosed`},
		{``, ``},
	}
	for _, c := range cases {
		if got := cleanCorePath(c[0]); got != c[1] {
			t.Errorf("cleanCorePath(%q) = %q, want %q", c[0], got, c[1])
		}
	}
}

func TestCheckCorePath(t *testing.T) {
	if err := checkCorePath(""); err != nil {
		t.Fatalf("empty path must pass (FindCore fallback): %v", err)
	}
	if err := checkCorePath("wisteria-client"); err != nil {
		t.Fatalf("bare name must resolve via PATH later: %v", err)
	}
	if err := checkCorePath(filepath.Join(t.TempDir(), "no-such-core.exe")); err == nil {
		t.Fatal("a missing file with separators must fail")
	} else if !strings.Contains(err.Error(), "Настройки") {
		t.Fatalf("error should point at the settings page, got: %v", err)
	}
	exe := filepath.Join(t.TempDir(), "core.exe")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := checkCorePath(exe); err != nil {
		t.Fatalf("existing file must pass: %v", err)
	}
}

func TestDirWritable(t *testing.T) {
	dir := t.TempDir()
	if !dirWritable(dir) {
		t.Fatal("a fresh temp dir must be writable")
	}
	if !dirWritable(BinDir()) {
		t.Fatal("BinDir must always resolve to a writable location")
	}
	if runtime.GOOS == "windows" {
		return // directory write-permission bits are a POSIX concept
	}
	ro := filepath.Join(dir, "ro")
	if err := os.Mkdir(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(ro, 0o755) // so t.TempDir cleanup can remove it
	if dirWritable(ro) {
		t.Fatal("a read-only dir must not be reported writable")
	}
}
