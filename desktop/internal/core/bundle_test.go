package core

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

var pub32 = base64.StdEncoding.EncodeToString(make([]byte, 32))

// mintBundle builds a link the same way cmd/wisteria-peer does:
// base64url(JSON) unpadded after the scheme.
func mintBundle(t *testing.T, v interface{}) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return "wisteria://" + base64.RawURLEncoding.EncodeToString(raw)
}

func TestParseLinkBundleLinkVariant(t *testing.T) {
	link := mintBundle(t, bundlePayload{
		V: 1,
		Servers: []bundleServer{
			{EP: "203.0.113.10:8443", Pub: base64.StdEncoding.EncodeToString(make([]byte, 32))},
			{EP: "203.0.113.20:8443", Pub: base64.StdEncoding.EncodeToString(make([]byte, 32))},
		},
		Name: "testpeer",
		Key:  base64.RawURLEncoding.EncodeToString(make([]byte, 32)),
	})
	profs, err := ParseLinkAll(link)
	if err != nil {
		t.Fatal(err)
	}
	if len(profs) != 2 {
		t.Fatalf("expanded to %d profiles, want 2", len(profs))
	}
	if profs[0].Name != "testpeer · 1" || profs[1].Name != "testpeer · 2" {
		t.Fatalf("names: %q %q", profs[0].Name, profs[1].Name)
	}
	if profs[0].Host != "203.0.113.10" || profs[0].Port != 8443 {
		t.Fatalf("server0: %s:%d", profs[0].Host, profs[0].Port)
	}
	// The link variant is a credential: the client key must land on every
	// expanded profile and be the same identity everywhere.
	if profs[0].ClientPriv == "" || profs[0].ClientPriv != profs[1].ClientPriv {
		t.Fatal("client key missing or differs across the bundle")
	}
	if profs[0].Fields["pub"] == "" || profs[1].Fields["pub"] == "" {
		t.Fatal("pinned server pub missing")
	}
	// Raw is dropped for multi bundles (store dedupe would collapse them).
	if profs[0].Raw != "" {
		t.Fatal("multi bundle must not carry Raw")
	}
	// ParseLink (single-profile API) refuses a multi bundle loudly.
	if _, err := ParseLink(link); err == nil {
		t.Fatal("ParseLink must refuse a multi-server bundle")
	}
}

func TestParseLinkBundleInviteVariant(t *testing.T) {
	link := mintBundle(t, bundlePayload{
		V: 1,
		Servers: []bundleServer{
			{EP: "fi.example.net:8443", Pub: base64.StdEncoding.EncodeToString(make([]byte, 32))},
		},
		Name: "pool",
	})
	profs, err := ParseLinkAll(link)
	if err != nil {
		t.Fatal(err)
	}
	if len(profs) != 1 || profs[0].Name != "pool" {
		t.Fatalf("got %+v", profs)
	}
	if profs[0].ClientPriv != "" {
		t.Fatal("invite variant must not carry a client key (the store generates one)")
	}
	// Single-server bundles keep the raw link so store dedupe works.
	if profs[0].Raw != link {
		t.Fatalf("raw = %q, want the original link", profs[0].Raw)
	}
	// And ParseLink handles it directly.
	p, err := ParseLink(link)
	if err != nil || p.Name != "pool" {
		t.Fatalf("ParseLink: %+v err=%v", p, err)
	}
}

func TestParseLinkBundleLegacySchemeAndPaddedBody(t *testing.T) {
	body := base64.URLEncoding.EncodeToString([]byte(
		`{"v":1,"servers":[{"ep":"h1.example:1","pub":""}],"name":"n"}`))
	profs, err := ParseLinkAll("WYSTERIA://" + body)
	if err != nil {
		t.Fatal(err)
	}
	if len(profs) != 1 || profs[0].Host != "h1.example" || profs[0].Port != 1 {
		t.Fatalf("got %+v", profs)
	}
}

// TLS transport (wave 4): per-server keys ride the JSON dialect. Absent
// keys keep the profile bare TCP (old links unchanged); a tls server row
// carries the transport, the optional SNI override and the insecure flag.
func TestParseLinkBundleTLSVariant(t *testing.T) {
	link := mintBundle(t, bundlePayload{
		V: 1,
		Servers: []bundleServer{
			{EP: "203.0.113.10:8443", Pub: pub32},
			{EP: "203.0.113.20:8443", Pub: pub32, Transport: "tcp"},
			{EP: "203.0.113.21:8443", Pub: pub32, Transport: "tls"},
			{EP: "203.0.113.22:8443", Pub: pub32, Transport: "tls", TLSSNI: "cdn.example", TLSInsecure: true},
		},
		Name: "tls-pool",
	})
	profs, err := ParseLinkAll(link)
	if err != nil {
		t.Fatal(err)
	}
	if len(profs) != 4 {
		t.Fatalf("expanded to %d profiles, want 4", len(profs))
	}
	for i, want := range []struct {
		tr, sni string
		insec   bool
	}{
		{"", "", false}, {"", "", false}, {"tls", "", false}, {"tls", "cdn.example", true},
	} {
		if profs[i].Transport != want.tr || profs[i].TLSSNI != want.sni || profs[i].TLSInsecure != want.insec {
			t.Fatalf("server %d: got transport=%q sni=%q insecure=%v, want %+v",
				i, profs[i].Transport, profs[i].TLSSNI, profs[i].TLSInsecure, want)
		}
	}
}

func TestParseLinkBundleTLSBrokenVariants(t *testing.T) {
	cases := map[string]string{
		"unknown transport": `{"v":1,"servers":[{"ep":"h:1","pub":"","transport":"quic"}]}`,
		"upper transport":   `{"v":1,"servers":[{"ep":"h:1","pub":"","transport":"TLS"}]}`,
		"sni without tls":   `{"v":1,"servers":[{"ep":"h:1","pub":"","tls_sni":"cdn.example"}]}`,
		"insec without tls": `{"v":1,"servers":[{"ep":"h:1","pub":"","tls_insecure":true}]}`,
		"ip sni":            `{"v":1,"servers":[{"ep":"h:1","pub":"","transport":"tls","tls_sni":"1.2.3.4"}]}`,
		"underscore sni":    `{"v":1,"servers":[{"ep":"h:1","pub":"","transport":"tls","tls_sni":"my_host.example"}]}`,
		"tcp with sni":      `{"v":1,"servers":[{"ep":"h:1","pub":"","transport":"tcp","tls_sni":"cdn.example"}]}`,
	}
	for name, payload := range cases {
		link := "wisteria://" + base64.RawURLEncoding.EncodeToString([]byte(payload))
		if _, err := ParseLinkAll(link); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestParseLinkBundleBrokenVariants(t *testing.T) {
	cases := map[string]string{
		"version missing": `{"servers":[{"ep":"h:1","pub":""}]}`,
		"version unknown": `{"v":2,"servers":[{"ep":"h:1","pub":""}]}`,
		"no servers":      `{"v":1,"servers":[]}`,
		"port missing":    `{"v":1,"servers":[{"ep":"h1.example","pub":""}]}`,
		"bad client key":  `{"v":1,"servers":[{"ep":"h:1","pub":""}],"key":"QUJD"}`,
		"bad server pub":  `{"v":1,"servers":[{"ep":"h:1","pub":"QUJD"}]}`,
	}
	for name, payload := range cases {
		link := "wisteria://" + base64.RawURLEncoding.EncodeToString([]byte(payload))
		if _, err := ParseLinkAll(link); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	// Base64 that does not decode to JSON must not fall into the classic
	// parser and become a junk profile (the regression from the field).
	if _, err := ParseLink("wisteria://YWJjZGVmZ2hpamtsbW5vcA"); err == nil {
		t.Error("base64 junk after the scheme must be rejected")
	}
}

func TestParseLinkClassicStillWorks(t *testing.T) {
	// Classic pinned-pub link with query/fragment: not a bundle.
	link := "wisteria://" + base64.StdEncoding.EncodeToString(make([]byte, 32)) +
		"@srv.example:8443?obf_mu=6.7&pad=64-1448#Classic"
	p, err := ParseLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if p.Type != TWisteria || p.Host != "srv.example" || p.Port != 8443 || p.Name != "Classic" {
		t.Fatalf("classic link broken: %+v", p)
	}
	if p.Fields["pub"] == "" || p.Fields["obf_mu"] != "6.7" {
		t.Fatalf("fields: %v", p.Fields)
	}
	// Junk without a port is no longer importable as a port-0 profile.
	if _, err := ParseLink("wisteria://justjunk"); err == nil {
		t.Fatal("junk body must be rejected")
	}
}

func TestSubscriptionWithBundleLine(t *testing.T) {
	bundle := mintBundle(t, bundlePayload{
		V: 1,
		Servers: []bundleServer{
			{EP: "a.example:1", Pub: pub32},
			{EP: "b.example:2", Pub: pub32},
		},
		Name: "sub-pool",
	})
	body := base64.StdEncoding.EncodeToString([]byte(bundle + "\n" + subHy2 + "\n"))
	res, err := ParseSubscriptionBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Profiles) != 3 { // 2 from the bundle + 1 hysteria2
		t.Fatalf("profiles = %d, want 3", len(res.Profiles))
	}
	if res.Profiles[0].Name != "sub-pool · 1" || res.Profiles[1].Host != "b.example" {
		t.Fatalf("bundle expansion wrong: %+v", res.Profiles[:2])
	}
}
