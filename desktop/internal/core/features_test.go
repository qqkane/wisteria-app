package core

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRankAutoCandidates(t *testing.T) {
	profiles := []Profile{
		{ID: AutoProfileID, Name: "Auto"},
		{ID: "dead", Name: "Dead", LatencyMs: -1},
		{ID: "slow", Name: "Slow", LatencyMs: 180},
		{ID: "untested", Name: "Untested", LatencyMs: 0},
		{ID: "fast", Name: "Fast", LatencyMs: 25},
	}
	ranked := rankAutoCandidates(profiles)
	if len(ranked) != 4 {
		t.Fatalf("expected 4 candidates, got %d", len(ranked))
	}
	want := []string{"fast", "slow", "untested", "dead"}
	for i, id := range want {
		if ranked[i].ID != id {
			t.Errorf("ranked[%d] = %q, want %q", i, ranked[i].ID, id)
		}
	}
}

func TestPickNextAutoProfileRotation(t *testing.T) {
	svc := NewService(Dirs{Data: t.TempDir(), Runtime: t.TempDir()}, NewLogRing(100), nil)
	svc.SetProfileProvider(func() []Profile {
		return []Profile{
			{ID: "p1", Name: "One", LatencyMs: 20},
			{ID: "p2", Name: "Two", LatencyMs: 40},
			{ID: "p3", Name: "Three", LatencyMs: 60},
		}
	})
	svc.mu.Lock()
	svc.autoMode = true
	svc.autoTried = map[string]bool{"p1": true}
	svc.mu.Unlock()

	n1, ok := svc.pickNextAutoProfile("p1")
	if !ok || n1.ID != "p2" {
		t.Fatalf("first failover = (%q, %v), want p2", n1.ID, ok)
	}
	n2, ok := svc.pickNextAutoProfile("p2")
	if !ok || n2.ID != "p3" {
		t.Fatalf("second failover = (%q, %v), want p3", n2.ID, ok)
	}
	// All exhausted -> resets round, avoiding the just-failed p3
	n3, ok := svc.pickNextAutoProfile("p3")
	if !ok || n3.ID != "p1" {
		t.Fatalf("wrapped failover = (%q, %v), want p1", n3.ID, ok)
	}
}

func TestFetchSubscriptionWithDeviceHeaders(t *testing.T) {
	var gotID, gotName, gotOS string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = r.Header.Get("X-Device-ID")
		gotName = r.Header.Get("X-Device-Name")
		gotOS = r.Header.Get("X-Device-OS")
		if gotID == "blocked-hwid" {
			http.Error(w, "device limit reached (1/1)", http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte("vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls#TestNode\n"))
	}))
	defer srv.Close()

	res, err := FetchSubscriptionWithDevice(srv.URL, 5*time.Second, DeviceHeaders{
		ID:   "0123456789abcdef0123456789abcdef",
		Name: "workstation",
		OS:   "linux",
	})
	if err != nil {
		t.Fatalf("FetchSubscriptionWithDevice failed: %v", err)
	}
	if len(res.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(res.Profiles))
	}
	if gotID != "0123456789abcdef0123456789abcdef" || gotName != "workstation" || gotOS != "linux" {
		t.Fatalf("unexpected headers: id=%q name=%q os=%q", gotID, gotName, gotOS)
	}

	_, err = FetchSubscriptionWithDevice(srv.URL, 5*time.Second, DeviceHeaders{ID: "blocked-hwid"})
	if err == nil || !strings.Contains(err.Error(), "device limit reached") {
		t.Fatalf("expected 403 device limit error, got %v", err)
	}
}

func TestLinuxShortcutsAndAutostart(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "share"))
	t.Setenv("HOME", tmp)
	if err := os.MkdirAll(filepath.Join(tmp, "Desktop"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := SetAutostart(true); err != nil {
		t.Fatalf("SetAutostart(true): %v", err)
	}
	if !AutostartEnabled() {
		t.Fatal("AutostartEnabled() should be true")
	}
	if err := SetAutostart(false); err != nil {
		t.Fatalf("SetAutostart(false): %v", err)
	}
	if AutostartEnabled() {
		t.Fatal("AutostartEnabled() should be false after disable")
	}

	deskPath, err := InstallLinuxShortcuts(true)
	if err != nil {
		t.Fatalf("InstallLinuxShortcuts(true): %v", err)
	}
	if _, err := os.Stat(deskPath); err != nil {
		t.Fatalf("desktop shortcut missing at %s: %v", deskPath, err)
	}
	menuPath := filepath.Join(tmp, "share", "applications", "wisteria-desktop.desktop")
	data, err := os.ReadFile(menuPath)
	if err != nil {
		t.Fatalf("menu shortcut missing at %s: %v", menuPath, err)
	}
	if !strings.Contains(string(data), "Icon=wisteria-desktop") {
		t.Fatalf("desktop entry missing Icon field:\n%s", string(data))
	}
}
