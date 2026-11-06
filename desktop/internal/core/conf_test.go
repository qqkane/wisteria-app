package core

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWisteriaTOMLRender(t *testing.T) {
	p := Profile{
		ID: "t1", Type: TWisteria, Name: "n", Host: "s.example", Port: 47823,
		ClientPriv: "PRIVKEYBASE64==",
		Fields: map[string]string{
			"pub": "PUBKEYBASE64==", "min_pad": "64", "max_pad": "1448",
			"mimic": "1", "filler_min": "80", "filler_max": "400",
		},
	}
	out, err := WisteriaTOML(p, "127.0.0.1:2080", "cache.cache")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`socks5_listen = "127.0.0.1:2080"`,
		`endpoint   = "s.example:47823"`,
		`public_key = "PUBKEYBASE64=="`,
		`static_private       = "PRIVKEYBASE64=="`,
		`server_static_public = "PUBKEYBASE64=="`,
		"min_pad = 64", "max_pad = 1448", "mimic_tls = 1",
		"filler_min = 80", "filler_max = 400",
		`mode = "global"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "jitter_micro_ms") {
		t.Errorf("empty fields must not be emitted:\n%s", out)
	}
}

func TestWisteriaTOMLRequiresKey(t *testing.T) {
	p := Profile{ID: "t2", Type: TWisteria, Host: "s", Port: 1}
	if _, err := WisteriaTOML(p, "127.0.0.1:2080", ""); err == nil {
		t.Fatal("missing client key must error")
	}
}

// The renderer is the last line of defense against TOML injection: the
// manual form and a hand-edited store reach it without the share-link
// parser, so it re-checks every numeric obfuscation field itself.
func TestWisteriaTOMLRejectsBadFields(t *testing.T) {
	p := Profile{ID: "t3", Type: TWisteria, Host: "s.example", Port: 47823,
		ClientPriv: "PRIVKEYBASE64==",
		Fields:     map[string]string{"obf_mu": "6.7\n[routing]\nmode = \"direct\""}}
	out, err := WisteriaTOML(p, "127.0.0.1:2080", "")
	if err == nil {
		t.Fatalf("injected field must be rejected, got:\n%s", out)
	}
}

// TLS profile (wave 4): [server] and the [[servers]] row both carry
// transport = "tls", the SNI override and the insecure flag land in the
// pool row only when non-default.
func TestWisteriaTOMLTLSRender(t *testing.T) {
	p := Profile{ID: "t4", Type: TWisteria, Host: "s.example", Port: 47823,
		ClientPriv: "PRIVKEYBASE64==", Transport: "tls", TLSSNI: "cdn.example", TLSInsecure: true,
		Fields: map[string]string{"pub": "PUBKEYBASE64=="}}
	out, err := WisteriaTOML(p, "127.0.0.1:2080", "cache.cache")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`[server]`, `endpoint  = "s.example:47823"`, `transport = "tls"`,
		`public_key = "PUBKEYBASE64=="`,
		`transport  = "tls"`, `tls_sni    = "cdn.example"`, `tls_insecure = true`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// tls_sni equal to the endpoint host is the loader's default — omit.
	p.TLSSNI = "s.example"
	out2, err := WisteriaTOML(p, "127.0.0.1:2080", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out2, "tls_sni") {
		t.Errorf("tls_sni == endpoint host must be omitted:\n%s", out2)
	}
	// A TLS profile without a pinned pub still needs the [[servers]] row:
	// the legacy [server] block has no tls_sni/tls_insecure fields.
	p.TLSSNI = "cdn.example"
	delete(p.Fields, "pub")
	out3, err := WisteriaTOML(p, "127.0.0.1:2080", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out3, "[[servers]]") || !strings.Contains(out3, `transport  = "tls"`) {
		t.Errorf("TLS without pub must still render the pool row:\n%s", out3)
	}
	if strings.Contains(out3, "public_key") {
		t.Errorf("no pub — no public_key row:\n%s", out3)
	}
}

// A tcp profile must render byte-identical to the pre-wave-4 output: the
// generated config is compared against expected text in tests and old
// profiles must not silently change shape after an update.
func TestWisteriaTOMLTCPSByteIdentical(t *testing.T) {
	p := Profile{ID: "t5", Type: TWisteria, Host: "s.example", Port: 47823,
		ClientPriv: "PRIV", Transport: "tcp",
		Fields: map[string]string{"pub": "PUB"}}
	out, err := WisteriaTOML(p, "127.0.0.1:2080", "cache.cache")
	if err != nil {
		t.Fatal(err)
	}
	want := `[client]
socks5_listen = "127.0.0.1:2080"

[server]
endpoint  = "s.example:47823"
transport = "tcp"

[[servers]]
endpoint   = "s.example:47823"
weight     = 1
public_key = "PUB"

[crypto]
pattern              = "auto"
static_private       = "PRIV"
server_static_public = "PUB"

[relay]
mode = "direct"
selection = "latency"
backoff_initial_ms = 500
backoff_max_ms = 30000
probe_interval_sec = 30
probe_timeout_sec = 3

[obfuscation]
# server defaults

[routing]
mode = "global"
`
	if out != want {
		t.Fatalf("tcp output changed:\n got: %q\nwant: %q", out, want)
	}
	// The explicit "tcp" spelling normalizes onto the zero value.
	p.Transport = ""
	out2, err := WisteriaTOML(p, "127.0.0.1:2080", "cache.cache")
	if err != nil || out2 != want {
		t.Fatalf("tcp/empty must render identically (err=%v)", err)
	}
}

// Build → import → same TLS fields: the TOML import must round-trip the
// wave-4 fields without loss, including the tcp → zero-value normalization.
func TestWisteriaTOMLTLSRoundTrip(t *testing.T) {
	p := Profile{ID: "t6", Type: TWisteria, Host: "s.example", Port: 47823,
		ClientPriv: "PRIVKEYBASE64==", Transport: "tls", TLSSNI: "cdn.example", TLSInsecure: true,
		Fields: map[string]string{"pub": "PUBKEYBASE64=="}}
	out, err := WisteriaTOML(p, "127.0.0.1:2080", "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseWisteriaTOML(out)
	if err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if got.Transport != "tls" || got.TLSSNI != "cdn.example" || !got.TLSInsecure {
		t.Fatalf("tls fields lost: %+v", got)
	}
	// tcp profile: transport comes back as the zero value.
	p.Transport, p.TLSSNI, p.TLSInsecure = "", "", false
	out2, err := WisteriaTOML(p, "127.0.0.1:2080", "")
	if err != nil {
		t.Fatal(err)
	}
	got2, err := ParseWisteriaTOML(out2)
	if err != nil {
		t.Fatal(err)
	}
	if got2.Transport != "" || got2.TLSSNI != "" || got2.TLSInsecure {
		t.Fatalf("tcp must import as the zero value: %+v", got2)
	}
}

// The renderer re-checks the TLS fields like it re-checks the obf numbers:
// a broken combo or a bad SNI from the manual form / hand-edited store is a
// render error, not a silently degraded config.
func TestWisteriaTOMLRejectsBadTLS(t *testing.T) {
	base := Profile{ID: "t7", Type: TWisteria, Host: "s.example", Port: 47823,
		ClientPriv: "PRIV", Fields: map[string]string{"pub": "PUB"}}
	for name, mut := range map[string]func(*Profile){
		"sni-without-tls":   func(p *Profile) { p.TLSSNI = "cdn.example" },
		"insec-without-tls": func(p *Profile) { p.TLSInsecure = true },
		"ip-sni":            func(p *Profile) { p.Transport, p.TLSSNI = "tls", "1.2.3.4" },
		"underscore-sni":    func(p *Profile) { p.Transport, p.TLSSNI = "tls", "my_host.example" },
		"bad-transport":     func(p *Profile) { p.Transport = "quic" },
	} {
		p := base
		mut(&p)
		if out, err := WisteriaTOML(p, "127.0.0.1:2080", ""); err == nil {
			t.Errorf("%s: must be rejected, got:\n%s", name, out)
		}
	}
}

func singBoxOut(t *testing.T, p Profile) map[string]any {
	t.Helper()
	raw, err := SingBoxConfig(p, SingBoxOptions{MixedPort: 2080})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("bad json: %v\n%s", err, raw)
	}
	// inbound sanity
	ib := doc["inbounds"].([]any)[0].(map[string]any)
	if ib["listen_port"].(float64) != 2080 || ib["type"] != "mixed" {
		t.Fatalf("inbound: %v", ib)
	}
	outs := doc["outbounds"].([]any)
	if len(outs) != 2 || doc["route"].(map[string]any)["final"] != "proxy" {
		t.Fatalf("outbounds/route: %v", doc)
	}
	return outs[0].(map[string]any)
}

func TestSingBoxOptions(t *testing.T) {
	p := Profile{ID: "o1", Type: TTrojan, Host: "t.example", Port: 443,
		Fields: map[string]string{"password": "pw"}}
	raw, err := SingBoxConfig(p, SingBoxOptions{
		MixedPort: 21080, Listen: "0.0.0.0", LogLevel: "debug",
		BypassLAN: true, RemoteDNS: "tcp://8.8.8.8",
	})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["log"].(map[string]any)["level"] != "debug" {
		t.Fatalf("log level: %v", doc["log"])
	}
	ib := doc["inbounds"].([]any)[0].(map[string]any)
	if ib["listen"] != "0.0.0.0" || ib["listen_port"].(float64) != 21080 {
		t.Fatalf("inbound: %v", ib)
	}
	route := doc["route"].(map[string]any)
	rules := route["rules"].([]any)
	r0 := rules[0].(map[string]any)
	if r0["ip_is_private"] != true || r0["outbound"] != "direct" {
		t.Fatalf("bypass LAN rule: %v", rules)
	}
	if route["default_domain_resolver"] != "remote" {
		t.Fatalf("dns resolver: %v", route)
	}
	dns := doc["dns"].(map[string]any)
	if dns["servers"].([]any)[0].(map[string]any)["address"] != "tcp://8.8.8.8" {
		t.Fatalf("dns: %v", dns)
	}

	// Defaults: no rules, no dns block, listen 127.0.0.1, level warn.
	raw2, _ := SingBoxConfig(p, SingBoxOptions{MixedPort: 2080})
	var doc2 map[string]any
	json.Unmarshal(raw2, &doc2)
	if _, has := doc2["dns"]; has {
		t.Fatalf("dns block must be absent by default")
	}
	r2 := doc2["route"].(map[string]any)
	if _, has := r2["rules"]; has {
		t.Fatalf("route rules must be absent by default")
	}
	if doc2["inbounds"].([]any)[0].(map[string]any)["listen"] != "127.0.0.1" {
		t.Fatalf("default listen")
	}
}

func TestSingBoxVLESSRealityWS(t *testing.T) {
	p := Profile{
		ID: "v1", Type: TVLESS, Host: "example.com", Port: 443,
		Fields: map[string]string{
			"uuid": "b831381d-6324-4d53-ad4f-8cda48b30811", "security": "reality",
			"sni": "www.microsoft.com", "public_key": "PBK", "short_id": "SID",
			"fingerprint": "chrome", "transport": "ws", "path": "/ws",
		},
	}
	o := singBoxOut(t, p)
	if o["type"] != "vless" || o["uuid"] != p.Fields["uuid"] || o["server"] != "example.com" {
		t.Fatalf("out: %v", o)
	}
	tls := o["tls"].(map[string]any)
	if tls["server_name"] != "www.microsoft.com" {
		t.Fatalf("tls: %v", tls)
	}
	real := tls["reality"].(map[string]any)
	if real["public_key"] != "PBK" || real["short_id"] != "SID" {
		t.Fatalf("reality: %v", real)
	}
	tr := o["transport"].(map[string]any)
	if tr["type"] != "ws" || tr["path"] != "/ws" {
		t.Fatalf("transport: %v", tr)
	}
}

func TestSingBoxSSAndTrojan(t *testing.T) {
	ss := Profile{ID: "s1", Type: TSS, Host: "1.2.3.4", Port: 8388,
		Fields: map[string]string{"method": "aes-256-gcm", "password": "pw"}}
	o := singBoxOut(t, ss)
	if o["type"] != "shadowsocks" || o["method"] != "aes-256-gcm" || o["password"] != "pw" {
		t.Fatalf("%v", o)
	}
	if _, has := o["tls"]; has {
		t.Fatalf("ss must not have tls block: %v", o)
	}

	tr := Profile{ID: "s2", Type: TTrojan, Host: "t.example", Port: 443,
		Fields: map[string]string{"password": "pw", "sni": "t.example", "transport": "tcp"}}
	o = singBoxOut(t, tr)
	if o["type"] != "trojan" {
		t.Fatalf("%v", o)
	}
	tls, has := o["tls"].(map[string]any)
	if !has || tls["server_name"] != "t.example" {
		t.Fatalf("trojan tls: %v", o)
	}
}

func TestSingBoxHysteria2(t *testing.T) {
	p := Profile{ID: "h1", Type: THysteria2, Host: "h.example", Port: 8443,
		Fields: map[string]string{"password": "auth", "sni": "h.example",
			"insecure": "1", "obfs": "salamander", "obfs_password": "ob"}}
	o := singBoxOut(t, p)
	if o["type"] != "hysteria2" || o["password"] != "auth" {
		t.Fatalf("%v", o)
	}
	if ob := o["obfs"].(map[string]any); ob["type"] != "salamander" || ob["password"] != "ob" {
		t.Fatalf("obfs: %v", ob)
	}
	tls := o["tls"].(map[string]any)
	if tls["insecure"] != true || tls["server_name"] != "h.example" {
		t.Fatalf("tls: %v", tls)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	p1, err := st.Add(Profile{Type: TWisteria, Name: "w", Host: "h", Port: 1, Raw: "wisteria://x@h:1"})
	if err != nil {
		t.Fatal(err)
	}
	if p1.ClientPriv == "" {
		t.Fatal("wisteria profile must get a generated client key")
	}
	if p1.ID == "" {
		t.Fatal("id must be assigned")
	}
	if _, err := st.Add(Profile{Type: TWisteria, Host: "h", Port: 1, Raw: "wisteria://x@h:1"}); err != nil {
		t.Fatal(err)
	}
	if len(st.List()) != 1 {
		t.Fatalf("dedupe by raw failed: %d", len(st.List()))
	}
	if err := st.SetActive(p1.ID); err != nil {
		t.Fatal(err)
	}
	// Reopen: persistence check.
	st2, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := st2.Active()
	if !ok || got.ID != p1.ID {
		t.Fatalf("active lost after reopen: %+v ok=%v", got, ok)
	}
	if err := st2.Delete(p1.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := st2.Active(); ok {
		t.Fatal("deleting the active profile must clear the pointer")
	}
}

func TestLogRing(t *testing.T) {
	r := NewLogRing(4) // clamped to the 16-line minimum
	for i := 0; i < 20; i++ {
		r.Add("app", "line")
	}
	all := r.Snapshot(0)
	if len(all) != 16 {
		t.Fatalf("capacity enforced: %d", len(all))
	}
	last := all[len(all)-1].Seq
	r.Add("core", "new")
	inc := r.Snapshot(last)
	if len(inc) != 1 || inc[0].Text != "new" {
		t.Fatalf("since-filter: %v", inc)
	}
}
