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

// TLS transport (wave 4): the three fields must survive a save/reopen cycle
// and the bare-TCP default must persist as the zero values (old stores have
// no keys at all and mean tcp).
func TestStoreTLSPersistence(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	tlsP, err := st.Add(Profile{Type: TWisteria, Name: "tls", Host: "s.example", Port: 1,
		Transport: "tls", TLSSNI: "cdn.example", TLSInsecure: true})
	if err != nil {
		t.Fatal(err)
	}
	// Explicit tcp normalizes onto the zero value: nothing new in the JSON.
	tcpP, err := st.Add(Profile{Type: TWisteria, Name: "tcp", Host: "t.example", Port: 2, Transport: "tcp"})
	if err != nil {
		t.Fatal(err)
	}
	if tcpP.Transport != "" || tcpP.TLSSNI != "" || tcpP.TLSInsecure {
		t.Fatalf("tcp must normalize onto the zero values: %+v", tcpP)
	}
	st2, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st2.Get(tlsP.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Transport != "tls" || got.TLSSNI != "cdn.example" || !got.TLSInsecure {
		t.Fatalf("tls fields lost after reopen: %+v", got)
	}
	if got, err = st2.Get(tcpP.ID); err != nil {
		t.Fatal(err)
	}
	if got.Transport != "" || got.TLSSNI != "" || got.TLSInsecure {
		t.Fatalf("tcp fields changed after reopen: %+v", got)
	}
}

// Every save path validates the TLS combo rules at the store choke point:
// a manual-form or hand-edited profile skips the link parsers but must not
// smuggle in tls_sni/tls_insecure without transport = "tls".
func TestStoreAddRejectsBadTLS(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	bad := []Profile{
		{Type: TWisteria, Host: "h", Port: 1, TLSSNI: "cdn.example"},
		{Type: TWisteria, Host: "h", Port: 1, TLSInsecure: true},
		{Type: TWisteria, Host: "h", Port: 1, Transport: "quic"},
		{Type: TWisteria, Host: "h", Port: 1, Transport: "tls", TLSSNI: "1.2.3.4"},
	}
	for i, p := range bad {
		if _, err := st.Add(p); err == nil {
			t.Errorf("case %d: expected an error", i)
		}
	}
}

func TestOpenStoreCorruptFileBackedUp(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(dir)
	if err == nil {
		t.Fatal("corrupt store must report an error")
	}
	backup := filepath.Join(dir, "profiles.json.corrupt")
	if b, rerr := os.ReadFile(backup); rerr != nil || string(b) != "{not json" {
		t.Fatalf("corrupt file not preserved as %s (err=%v)", backup, rerr)
	}
	// The store must be usable afterwards: the next save writes a fresh
	// profiles.json instead of dying on the removed original.
	if _, aerr := s.Add(Profile{Name: "n", Type: TVLESS, Host: "h", Port: 1}); aerr != nil {
		t.Fatalf("Add after corrupt open: %v", aerr)
	}
	if _, rerr := os.Stat(filepath.Join(dir, "profiles.json")); rerr != nil {
		t.Fatalf("profiles.json not re-created: %v", rerr)
	}
}
