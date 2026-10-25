package core

import "testing"

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
	f.Add("http://sub.example.com/list?token=1")
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
