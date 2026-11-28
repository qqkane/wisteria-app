package core

import (
	"encoding/base64"
	"strings"
	"testing"
)

// FuzzParseLinkAll fuzzes the share-link import path. Users paste links
// from chats, web pages and subscriptions, so this parser faces fully
// attacker-controlled text: any byte sequence must return an error or a
// profile list — never a panic, never an index-out-of-range, never an
// unbounded loop.
func FuzzParseLinkAll(f *testing.F) {
	f.Add("wisteria://x")
	f.Add("wisteria://user@host:1?key=abc")
	f.Add("wisteria-bundle://AAAA")
	f.Add("vless://uuid@host:443?type=ws#name")
	f.Add("vmess://eyJhZGQiOiJoIiwicG9ydCI6IjEifQ==")
	f.Add("ss://YWVzLTI1Ni1nY206cHc@a.b.c:8388#node")
	f.Add("hysteria2://pw@h:1/?insecure=1")
	f.Add("trojan://pw@h:1?sni=x")
	f.Add("other://sub/list?token=1")
	f.Add("://")
	f.Add("wisteria://host:99999999999999999999")
	f.Add("%zz%zz%zz")

	f.Fuzz(func(t *testing.T, line string) {
		profs, err := ParseLinkAll(line)
		if err != nil && len(profs) != 0 {
			t.Fatalf("error with non-empty result: %v, %d profiles", err, len(profs))
		}
		for _, p := range profs {
			if !Supported(p.Type) {
				t.Fatalf("parsed unsupported type %q", p.Type)
			}
			if p.Port < 0 || p.Port > 65535 {
				t.Fatalf("parsed out-of-range port %d", p.Port)
			}
		}
	})
}

func FuzzParseSubscriptionBody(f *testing.F) {
	plain := strings.Join([]string{
		"# comment",
		"// another comment",
		"vless://11111111-1111-1111-1111-111111111111@node-a:443?security=tls#NodeA",
		"trojan://secret@node-b:443?sni=localhost#NodeB",
		"garbage line",
	}, "\n")
	f.Add("")
	f.Add("<!DOCTYPE html><html><body>login</body></html>")
	f.Add(plain)
	f.Add(base64.StdEncoding.EncodeToString([]byte(plain)))
	f.Add(base64.RawURLEncoding.EncodeToString([]byte(plain)))
	f.Add("not-a-link\nstill-not-a-link")

	f.Fuzz(func(t *testing.T, body string) {
		res, err := ParseSubscriptionBody(body)
		if err != nil {
			if len(res.Profiles) != 0 {
				t.Fatalf("error %v with %d profiles", err, len(res.Profiles))
			}
			return
		}
		if len(res.Profiles) == 0 {
			t.Fatal("nil error with empty Profiles")
		}
		for _, p := range res.Profiles {
			if !Supported(p.Type) {
				t.Fatalf("unsupported profile type %q", p.Type)
			}
			if p.Port < 0 || p.Port > 65535 {
				t.Fatalf("out-of-range port %d", p.Port)
			}
		}
	})
}

func FuzzParseWisteriaBundle(f *testing.F) {
	pub := base64.StdEncoding.EncodeToString(make([]byte, 32))
	priv := base64.StdEncoding.EncodeToString(make([]byte, 32))
	validJSON := `{"v":1,"servers":[{"ep":"127.0.0.1:443","pub":"` + pub + `","transport":"tls","tls_sni":"localhost","tls_insecure":true}],"name":"bundle","key":"` + priv + `","lvl":"stealth"}`

	f.Add("")
	f.Add("wisteria://")
	f.Add("wisteria://not-base64***")
	f.Add("wisteria://" + base64.RawURLEncoding.EncodeToString([]byte(validJSON)))
	f.Add("wysteria://" + base64.URLEncoding.EncodeToString([]byte(validJSON)))
	f.Add("wisteria://" + base64.RawURLEncoding.EncodeToString([]byte(`{"v":2,"servers":[]}`)))
	f.Add("wisteria://" + base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"servers":[{"ep":"127.0.0.1:0","pub":"`+pub+`"}]}`)))

	f.Fuzz(func(t *testing.T, raw string) {
		profs, err := ParseWisteriaBundle(raw)
		if err != nil {
			if len(profs) != 0 {
				t.Fatalf("error %v with %d profiles", err, len(profs))
			}
			return
		}
		if len(profs) == 0 {
			t.Fatal("nil error with zero profiles")
		}
		for _, p := range profs {
			if p.Type != TWisteria {
				t.Fatalf("profile type = %q, want %q", p.Type, TWisteria)
			}
			if p.Host == "" {
				t.Fatal("empty Host in parsed bundle profile")
			}
			if p.Port <= 0 || p.Port > 65535 {
				t.Fatalf("invalid Port %d in parsed bundle profile", p.Port)
			}
		}
	})
}

func FuzzParseWisteriaTOML(f *testing.F) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	f.Add("")
	f.Add("[server]\nendpoint = \"127.0.0.1:48200\"\n[crypto]\nstatic_private = \"" + key + "\"\nserver_static_public = \"" + key + "\"\n")
	f.Add("[[servers]]\nendpoint = \"127.0.0.1:443\"\npublic_key = \"" + key + "\"\ntransport = \"tls\"\ntls_sni = \"localhost\"\ntls_insecure = true\n[crypto]\nstatic_private = \"" + key + "\"\n[obfuscation]\nmin_pad = 64\nmax_pad = 1448\n")
	f.Add("[server]\nendpoint = \"bad-endpoint\"\n[crypto]\nstatic_private = \"" + key + "\"\n")
	f.Add("[[[broken toml")

	f.Fuzz(func(t *testing.T, text string) {
		p, err := ParseWisteriaTOML(text)
		if err != nil {
			return
		}
		if p.Type != TWisteria {
			t.Fatalf("profile type = %q, want %q", p.Type, TWisteria)
		}
		if p.Host == "" {
			t.Fatal("empty Host in parsed TOML profile")
		}
		if p.Port < 0 || p.Port > 65535 {
			t.Fatalf("out-of-range Port %d in parsed TOML profile", p.Port)
		}
	})
}
