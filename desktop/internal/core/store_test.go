package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenStoreNormalizesLegacyTypeID(t *testing.T) {
	dir := t.TempDir()
	body := `{"profiles":[
		{"id":"p1","name":"legacy","type":"wysteria","host":"h1","port":1,"created_at":"2026-10-22T00:00:00Z"},
		{"id":"p2","name":"current","type":"wisteria","host":"h2","port":2,"created_at":"2026-10-23T00:00:00Z"},
		{"id":"p3","name":"other","type":"vless","host":"h3","port":3,"created_at":"2026-10-23T00:00:00Z"}
	],"active":"p1"}`
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Profiles) != 3 {
		t.Fatalf("profiles = %d", len(s.Profiles))
	}
	if s.Profiles[0].Type != TWisteria {
		t.Fatalf("legacy type not normalized: %q", s.Profiles[0].Type)
	}
	if !Supported(s.Profiles[0].Type) {
		t.Fatal("normalized profile must be Supported")
	}
	if s.Profiles[1].Type != TWisteria || s.Profiles[2].Type != TVLESS {
		t.Fatalf("untouched profiles changed: %q %q", s.Profiles[1].Type, s.Profiles[2].Type)
	}
}
