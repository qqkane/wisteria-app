package core

import (
	"encoding/json"
	"testing"
)

func TestParseRouteList(t *testing.T) {
	raw := `
# comment
example.com
// another comment
10.0.0.0/8, 1.2.3.4;;
*.bad.domain
MySite.ORG.
`
	domains, cidrs := parseRouteList(raw)
	if len(domains) != 2 || domains[0] != "example.com" || domains[1] != "mysite.org" {
		t.Fatalf("domains = %v", domains)
	}
	if len(cidrs) != 2 || cidrs[0] != "10.0.0.0/8" || cidrs[1] != "1.2.3.4" {
		t.Fatalf("cidrs = %v", cidrs)
	}
}

func TestEngineFor(t *testing.T) {
	s := Settings{}
	s.defaults() // all per-protocol engines fall back to xray (the v1.1.3 default)
	for _, p := range []ProfileType{TSS, TVMess, TVLESS, TTrojan} {
		if got := s.EngineFor(p); got != "xray" {
			t.Fatalf("%s: got %s", p, got)
		}
	}
	s2 := Settings{MPCore: "xray"}
	s2.defaults() // legacy default propagates
	if got := s2.EngineFor(TVLESS); got != "xray" {
		t.Fatalf("legacy default: got %s", got)
	}
	s3 := Settings{MPCore: "xray", CoreVless: "sing-box", EnginesXrayMigration: true}
	s3.defaults() // post-migration: a deliberate per-proto choice is kept
	if got := s3.EngineFor(TVLESS); got != "sing-box" {
		t.Fatalf("per-proto override: got %s", got)
	}
	if got := s3.EngineFor(THysteria2); got != "sing-box" {
		t.Fatalf("hysteria2 pinned: got %s", got)
	}
	if got := s3.EngineFor(TWisteria); got != "sing-box" {
		t.Fatalf("wisteria out of scope: got %s", got)
	}
}

func TestXrayNeedsTransportSecurity(t *testing.T) {
	mk := func(host string, fields map[string]string) Profile {
		return Profile{Type: TVLESS, Host: host, Fields: fields}
	}
	cases := []struct {
		name string
		p    Profile
		want bool
	}{
		{"plain vless to a public domain", mk("seiercore.online", map[string]string{"security": "none"}), true},
		{"plain vless to a public IP", mk("8.8.8.8", map[string]string{"security": "none"}), true},
		{"plain vless to a LAN IP", mk("192.168.1.5", map[string]string{"security": "none"}), false},
		{"plain vless to localhost", mk("localhost", map[string]string{"security": "none"}), false},
		{"plain vless to a dotless name", mk("vpn", map[string]string{"security": "none"}), false},
		{"plain vless to .local", mk("box.local", map[string]string{"security": "none"}), false},
		{"vless with TLS", mk("seiercore.online", map[string]string{"security": "tls"}), false},
		{"vless with REALITY", mk("seiercore.online", map[string]string{"security": "reality"}), false},
		{"vless with a reality public key", mk("seiercore.online", map[string]string{"public_key": "k"}), false},
	}
	for _, c := range cases {
		if got := XrayNeedsTransportSecurity(c.p); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	// Only VLESS has the plaintext guard: VMess encrypts its own payload
	// and the renderer always gives Trojan TLS settings.
	for _, typ := range []ProfileType{TSS, TVMess, TTrojan} {
		p := Profile{Type: typ, Host: "seiercore.online", Fields: map[string]string{}}
		if XrayNeedsTransportSecurity(p) {
			t.Errorf("%s must not need TLS fallback", typ)
		}
	}
}

func TestSingBoxConfigRouteRules(t *testing.T) {
	p := Profile{ID: "t", Type: TVLESS, Host: "srv.example", Port: 443, Fields: map[string]string{"uuid": "u"}}
	conf, err := SingBoxConfig(p, SingBoxOptions{
		MixedPort:     2080,
		DirectDomains: []string{"example.com"},
		DirectCIDRs:   []string{"10.0.0.0/8"},
		BlockDomains:  []string{"ads.example"},
		BlockCIDRs:    []string{"1.2.3.0/24"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Outbounds []struct {
			Tag  string `json:"tag"`
			Type string `json:"type"`
		} `json:"outbounds"`
		Route struct {
			Rules []map[string]any `json:"rules"`
		} `json:"route"`
	}
	if err := json.Unmarshal(conf, &doc); err != nil {
		t.Fatal(err)
	}
	foundBlock := false
	for _, o := range doc.Outbounds {
		if o.Tag == "block" && o.Type == "block" {
			foundBlock = true
		}
	}
	if !foundBlock {
		t.Fatal("no block outbound")
	}
	want := 4 // direct d/c + block d/c
	if len(doc.Route.Rules) != want {
		t.Fatalf("rules = %d, want %d: %v", len(doc.Route.Rules), want, doc.Route.Rules)
	}
}

func TestXrayConfigRouteRules(t *testing.T) {
	p := Profile{ID: "t", Type: TVLESS, Host: "srv.example", Port: 443, Fields: map[string]string{"uuid": "u"}}
	conf, err := XrayConfig(p, XrayOptions{
		SocksPort:    2080,
		BlockDomains: []string{"ads.example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(conf)
	if !contains(s, `"domain:ads.example"`) {
		t.Fatalf("no block domain rule in %s", s)
	}
	if !contains(s, `"protocol": "blackhole"`) {
		t.Fatal("no blackhole outbound")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestTunConfigStackAndStrict(t *testing.T) {
	conf, err := TunConfig(TunOptions{SocksAddr: "127.0.0.1:2080", Stack: "system", StrictRoute: true})
	if err != nil {
		t.Fatal(err)
	}
	s := string(conf)
	if !contains(s, `"stack": "system"`) || !contains(s, `"strict_route": true`) {
		t.Fatalf("stack/strict not in config: %s", s)
	}
	conf, err = TunConfig(TunOptions{SocksAddr: "127.0.0.1:2080", Stack: "bogus"})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(conf), `"stack": "mixed"`) {
		t.Fatal("bad stack did not fall back to mixed")
	}
}
