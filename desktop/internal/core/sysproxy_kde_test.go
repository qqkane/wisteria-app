//go:build !windows

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeKDETools writes kwriteconfig/kreadconfig shims that store kioslaverc
// as plain "key=value" lines under $KDE_TEST_HOME. Deterministic on CI,
// where the real KDE tools are not installed.
func fakeKDETools(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	home := t.TempDir()
	t.Setenv("KDE_TEST_HOME", home)
	write := "#!/bin/sh\n" + `# kwriteconfig shim: [--notify] --file F --group G --key K (value|--delete)
key=""; group=""; val=""; del=0
while [ $# -gt 0 ]; do
  case "$1" in
    --key) key="$2"; shift 2;;
    --group) group="$2"; shift 2;;
    --file) shift 2;;
    --notify) shift;;
    --delete) del=1; shift;;
    *) val="$1"; shift;;
  esac
done
f="$KDE_TEST_HOME/kioslaverc"
touch "$f"
if [ "$del" = "1" ]; then
  grep -v "^${key}=" "$f" > "$f.tmp" 2>/dev/null || true
  mv "$f.tmp" "$f"
elif [ -n "$val" ]; then
  grep -v "^${key}=" "$f" > "$f.tmp" 2>/dev/null || true
  echo "${key}=${val}" >> "$f.tmp" && mv "$f.tmp" "$f"
fi
`
	read := "#!/bin/sh\n" + `# kreadconfig shim: --file F --group G --key K
key=""
while [ $# -gt 0 ]; do
  case "$1" in
    --key) key="$2"; shift 2;;
    *) shift;;
  esac
done
grep "^${key}=" "$KDE_TEST_HOME/kioslaverc" 2>/dev/null | head -1 | cut -d= -f2-
`
	// Every name variant ktool() probes: 6 first, then 5, then bare. With
	// all of them shimmed, LookPath can never reach the real KDE tools and
	// the tests cannot touch the developer's own kioslaverc.
	for name, body := range map[string]string{
		"kwriteconfig5": write, "kwriteconfig6": write, "kwriteconfig": write,
		"kreadconfig5": read, "kreadconfig6": read, "kreadconfig": read,
	} {
		p := filepath.Join(bin, name)
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return home
}

func TestKDESetAndClearProxy(t *testing.T) {
	fakeKDETools(t)
	// Pin the desktop so the dispatch picks the KDE backend regardless of
	// the machine running the tests.
	t.Setenv("KDE_FULL_SESSION", "1")

	if err := SetSystemProxy(true, "127.0.0.1:2081"); err != nil {
		t.Fatalf("set: %v", err)
	}
	mode := kdeRead(ktool("kreadconfig"), "mode", "Proxy Settings")
	if mode != "manual" {
		t.Fatalf("mode = %q, want manual", mode)
	}
	proxy := kdeRead(ktool("kreadconfig"), "httpProxy", "Proxy Settings")
	if proxy != "http://127.0.0.1:2081" {
		t.Fatalf("httpProxy = %q", proxy)
	}

	// Stale detection: same port → cleared; foreign port → untouched.
	if !kdeClearStale(2081) {
		t.Fatal("stale proxy on our port was not cleared")
	}
	if kdeRead(ktool("kreadconfig"), "mode", "Proxy Settings") != "none" {
		t.Fatal("mode not reset to none")
	}

	if err := SetSystemProxy(true, "127.0.0.1:2081"); err != nil {
		t.Fatalf("set again: %v", err)
	}
	if kdeClearStale(9999) {
		t.Fatal("foreign-port proxy must be left alone")
	}
	if mode := kdeRead(ktool("kreadconfig"), "mode", "Proxy Settings"); mode != "manual" {
		t.Fatalf("foreign-port clear changed mode to %q", mode)
	}
}

func TestKDEBackendPickedOnKDEDesktop(t *testing.T) {
	fakeKDETools(t)
	t.Setenv("KDE_FULL_SESSION", "1")

	b, err := pickProxyBackend()
	if err != nil {
		t.Fatalf("pickProxyBackend: %v", err)
	}
	if b.name != "kde" {
		t.Fatalf("backend = %q, want kde on a KDE session", b.name)
	}
	if !SystemProxySupported() {
		t.Fatal("SystemProxySupported must be true with a KDE backend present")
	}
}

func TestProxyBackendAbsentWithoutTools(t *testing.T) {
	bin := t.TempDir()
	// PATH without gsettings/kwriteconfig, but with the essentials.
	t.Setenv("PATH", "/usr/bin:/bin")
	if _, err := os.Stat("/usr/bin/gsettings"); err == nil {
		t.Skip("gsettings installed system-wide; isolation impossible")
	}
	_ = bin
	if _, ok := kdeBackend(); ok {
		t.Fatal("kde backend must be unavailable without kwriteconfig")
	}
	if _, ok := gnomeBackend(); ok {
		t.Fatal("gnome backend must be unavailable without gsettings")
	}
	if _, err := pickProxyBackend(); err == nil {
		t.Fatal("pickProxyBackend must fail with no backends")
	} else if !strings.Contains(err.Error(), "не поддерживается") {
		t.Fatalf("unexpected error: %v", err)
	}
}
