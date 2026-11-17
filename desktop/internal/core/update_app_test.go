package core

// App-bundle update layout tests: which asset the installer-less app picks
// for its own layout, and what happens when that layout cannot be written
// to (the AppImage squashfs mount case).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppAssetForLayout(t *testing.T) {
	cases := []struct {
		goos string
		img  string
		want string
	}{
		{"windows", `C:\Apps\Wisteria.exe`, "WisteriaDesktop-windows-amd64.zip"},
		{"linux", "", "WisteriaDesktop-linux-amd64.tar.gz"},
		{"linux", "/home/u/WisteriaDesktop.AppImage", "WisteriaDesktop-linux-x86_64.AppImage"},
		{"darwin", "", "WisteriaDesktop-linux-amd64.tar.gz"},
	}
	for _, c := range cases {
		if got := appAssetFor(c.goos, c.img); got != c.want {
			t.Errorf("appAssetFor(%q, %q) = %q, want %q", c.goos, c.img, got, c.want)
		}
	}
}

func TestAppImagePathOnlyExistingFile(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "WisteriaDesktop.AppImage")
	if err := os.WriteFile(real, []byte("img"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "subdir")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("APPIMAGE", real)
	if got := AppImagePath(); got != real {
		t.Errorf("AppImagePath() = %q, want %q", got, real)
	}
	t.Setenv("APPIMAGE", filepath.Join(dir, "gone.AppImage"))
	if got := AppImagePath(); got != "" {
		t.Errorf("missing image must not count, got %q", got)
	}
	t.Setenv("APPIMAGE", sub)
	if got := AppImagePath(); got != "" {
		t.Errorf("a directory is not an image, got %q", got)
	}
	t.Setenv("APPIMAGE", "   ")
	if got := AppImagePath(); got != "" {
		t.Errorf("blank APPIMAGE must not count, got %q", got)
	}
}

// The bug this guards: an AppImage reports os.Executable() inside its
// read-only squashfs mount, so staging there died with
// "переименование …: read-only file system" — a message with no way out.
func TestStageAppBinaryRefusesReadOnlyDir(t *testing.T) {
	src := filepath.Join(t.TempDir(), "WisteriaDesktop")
	if err := os.WriteFile(src, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "WisteriaDesktop")
	if err := os.WriteFile(dst, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := stageAppBinary(src, dst, "WisteriaDesktop-linux-x86_64.AppImage")
	if err == nil {
		t.Fatal("read-only install dir must be refused, not staged into")
	}
	if !strings.Contains(err.Error(), "только на чтение") {
		t.Errorf("error must name the problem, got: %v", err)
	}
	if !strings.Contains(err.Error(), "WisteriaDesktop-linux-x86_64.AppImage") {
		t.Errorf("error must name the asset to fetch by hand, got: %v", err)
	}
	data, readErr := os.ReadFile(dst)
	if readErr != nil || string(data) != "old" {
		t.Errorf("refused update must leave the running binary intact, got %q (%v)", data, readErr)
	}
}

// The same staging on a writable dir must work and leave the .old copy for
// the next start to clean up.
func TestStageAppBinaryWritesToWritableDir(t *testing.T) {
	src := filepath.Join(t.TempDir(), "WisteriaDesktop")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "WisteriaDesktop")
	if err := os.WriteFile(dst, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := stageAppBinary(src, dst, "WisteriaDesktop-linux-amd64.tar.gz"); err != nil {
		t.Fatalf("writable dir must stage cleanly: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "new" {
		t.Fatalf("staged file = %q (%v), want \"new\"", data, err)
	}
	if st, err := os.Stat(dst); err != nil || st.Mode().Perm()&0o111 == 0 {
		t.Errorf("staged app binary must stay executable, got %v (%v)", st.Mode(), err)
	}
	if _, err := os.Stat(dst + ".old"); err != nil {
		t.Errorf("previous binary should be kept as .old for rollback/cleanup: %v", err)
	}
}

func TestCleanupOldUpdateRemovesAppImageOld(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "WisteriaDesktop.AppImage")
	if err := os.WriteFile(img, []byte("img"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := img + ".old"
	if err := os.WriteFile(old, []byte("previous release"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPIMAGE", img)

	CleanupOldUpdate()

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("leftover %s must be cleaned up after a restart", old)
	}
	if _, err := os.Stat(img); err != nil {
		t.Errorf("the image itself must survive cleanup: %v", err)
	}
}
