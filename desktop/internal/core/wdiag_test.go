package core

import (
	"os"
	"testing"
)

func TestClassifyWisteriaLine(t *testing.T) {
	cases := []struct {
		line string
		kind string
	}{
		{"2026/10/23 12:00:00 CONNECTED via 1.2.3.4:443 - SOCKS5 proxy on 127.0.0.1:2080", "up"},
		{"2026/10/23 12:00:00 handshake refused: the server rejected the handshake — the link is stale or the peer is not authorized; an invite link without a key cannot connect", "blocked"},
		{"2026/10/23 12:00:00 cannot reach 1.2.3.4:443: dial tcp 1.2.3.4:443: connectex: A connection attempt failed", "nonet"},
		{"2026/10/23 12:00:00 session to 1.2.3.4:443 ended (timeout)", "down"},
		{"2026/10/23 12:00:00 retrying in 2.5s (attempt 3) - press d to stop, c to retry right now", "retry"},
		{"2026/10/23 12:00:00 health: rtt=87ms loss=0% pings=12", "health"},
		{"2026/10/23 12:00:00 client key in the config: base64", "info"},
		{"  hint line following a parent event", ""},
		{"2026/10/23 12:00:00 point your app at SOCKS5 127.0.0.1:2080", ""},
	}
	for _, c := range cases {
		ev, ok := classifyWisteriaLine(c.line)
		if c.kind == "" {
			if ok {
				t.Errorf("%q classified as %s, want skip", c.line, ev.Kind)
			}
			continue
		}
		if !ok || ev.Kind != c.kind {
			t.Errorf("%q classified as %q/%v, want %q", c.line, ev.Kind, ok, c.kind)
		}
		if ev.Text == "" {
			t.Errorf("%q: empty text", c.line)
		}
	}
}

func TestReadWisteriaStatus(t *testing.T) {
	// The core marshals ServerStatus without tags → field-name keys.
	path := t.TempDir() + "/status.json"
	content := `{
  "updated": 1730000000000000000,
  "servers": [
    {"Endpoint": "1.2.3.4:443", "Weight": 1, "PublicKey": "abc", "RTTMs": 87, "Reachable": true, "Failures": 0},
    {"Endpoint": "5.6.7.8:443", "Weight": 1, "PublicKey": "def", "RTTMs": 0, "Reachable": false, "Failures": 3}
  ]
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	servers, updated := readWisteriaStatus(path)
	if updated != 1730000000000000000 || len(servers) != 2 {
		t.Fatalf("servers=%+v updated=%d", servers, updated)
	}
	if servers[0].Endpoint != "1.2.3.4:443" || !servers[0].Reachable || servers[0].RTTMs != 87 {
		t.Fatalf("row0 = %+v", servers[0])
	}
	if servers[1].Reachable || servers[1].Failures != 3 {
		t.Fatalf("row1 = %+v", servers[1])
	}
	if _, upd := readWisteriaStatus(path + ".missing"); upd != 0 {
		t.Fatal("missing file must be silent")
	}
}
