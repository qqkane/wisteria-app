package core

import (
	"encoding/json"
	"testing"
)

func xrayOut(t *testing.T, p Profile, o XrayOptions) (map[string]any, map[string]any) {
	t.Helper()
	conf, err := XrayConfig(p, o)
	if err != nil {
		t.Fatalf("XrayConfig: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(conf, &m); err != nil {
		t.Fatalf("json: %v", err)
	}
	return m, m["outbounds"].([]any)[0].(map[string]any)
}

func TestXrayVLESSRealityWS(t *testing.T) {
	p := Profile{
		ID: "x1", Type: TVLESS, Host: "ex.com", Port: 443,
		Fields: map[string]string{
			"uuid": "u-1", "security": "reality", "sni": "cdn.example",
			"transport": "ws", "path": "/wspath", "public_key": "PK", "short_id": "AB",
			"flow": "xtls-rprx-vision",
		},
	}
	m, out := xrayOut(t, p, XrayOptions{SocksPort: 2080})
	if out["protocol"] != "vless" {
		t.Fatalf("protocol = %v", out["protocol"])
	}
	vnext := out["settings"].(map[string]any)["vnext"].([]any)[0].(map[string]any)
	if vnext["address"] != "ex.com" || vnext["port"] != float64(443) {
		t.Fatalf("vnext = %v", vnext)
	}
	ss := out["streamSettings"].(map[string]any)
	if ss["network"] != "ws" || ss["security"] != "reality" {
		t.Fatalf("stream = %v", ss)
	}
	rs := ss["realitySettings"].(map[string]any)
	if rs["publicKey"] != "PK" || rs["shortId"] != "AB" || rs["serverName"] != "cdn.example" {
		t.Fatalf("reality = %v", rs)
	}
	ws := ss["wsSettings"].(map[string]any)
	if ws["path"] != "/wspath" {
		t.Fatalf("ws = %v", ws)
	}
	in := m["inbounds"].([]any)[0].(map[string]any)
	if in["listen"] != "127.0.0.1" || in["port"] != float64(2080) {
		t.Fatalf("inbound = %v", in)
	}
}

func TestXrayTrojanTLSAndBypass(t *testing.T) {
	p := Profile{
		ID: "x2", Type: TTrojan, Host: "tr.net", Port: 443,
		Fields: map[string]string{"password": "pw"},
	}
	m, out := xrayOut(t, p, XrayOptions{SocksPort: 2080, BypassLAN: true, LogLevel: "warn"})
	if out["protocol"] != "trojan" {
		t.Fatalf("protocol = %v", out["protocol"])
	}
	ss := out["streamSettings"].(map[string]any)
	if ss["security"] != "tls" {
		t.Fatalf("trojan must be TLS, stream = %v", ss)
	}
	routing := m["routing"].(map[string]any)
	rules := routing["rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("rules = %v", rules)
	}
	ips, ok := rules[0].(map[string]any)["ip"].([]any)
	if !ok || len(ips) < 5 {
		t.Fatalf("private cidrs missing: %v", rules[0])
	}
	if len(ips) < 5 {
		t.Fatalf("private cidrs missing: %v", ips)
	}
	if m["log"].(map[string]any)["loglevel"] != "warning" {
		t.Fatalf("loglevel = %v", m["log"])
	}
}

func TestXraySSVMess(t *testing.T) {
	ss := Profile{ID: "x3", Type: TSS, Host: "1.2.3.4", Port: 8388,
		Fields: map[string]string{"method": "aes-256-gcm", "password": "p"}}
	_, out := xrayOut(t, ss, XrayOptions{SocksPort: 2080})
	if out["protocol"] != "shadowsocks" {
		t.Fatalf("protocol = %v", out["protocol"])
	}
	vm := Profile{ID: "x4", Type: TVMess, Host: "1.2.3.4", Port: 443,
		Fields: map[string]string{"uuid": "u", "tls": "1", "sni": "s.example"}}
	_, out = xrayOut(t, vm, XrayOptions{SocksPort: 2080, RemoteDNS: "tcp://8.8.8.8"})
	if out["protocol"] != "vmess" {
		t.Fatalf("protocol = %v", out["protocol"])
	}
	if _, ok := out["streamSettings"].(map[string]any)["tlsSettings"]; !ok {
		t.Fatalf("vmess tls=1 must render tlsSettings")
	}
}

func TestXrayUnsupported(t *testing.T) {
	p := Profile{ID: "x5", Type: THysteria2, Host: "h", Port: 443,
		Fields: map[string]string{"password": "p"}}
	if _, err := XrayConfig(p, XrayOptions{SocksPort: 2080}); err == nil {
		t.Fatal("hysteria2 must be rejected by the xray renderer")
	}
}

func TestTunConfigModern(t *testing.T) {
	conf, err := TunConfig(TunOptions{
		SocksAddr: "127.0.0.1:2080",
		ServerIPs: []string{"203.0.113.7", "198.51.100.9"},
		BypassLAN: true, ModernDNS: true, LogLevel: "warn",
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(conf, &m)
	in := m["inbounds"].([]any)[0].(map[string]any)
	if in["type"] != "tun" || in["auto_route"] != true {
		t.Fatalf("inbound = %v", in)
	}
	if _, hasSniff := in["sniff"]; hasSniff {
		t.Fatal("modern config must not carry the inbound sniff option")
	}
	rules := m["route"].(map[string]any)["rules"].([]any)
	first := rules[0].(map[string]any)
	if first["action"] != "hijack-dns" {
		t.Fatalf("dns rule = %v", first)
	}
	excl := rules[1].(map[string]any)
	if excl["outbound"] != "direct" {
		t.Fatalf("server rule = %v", excl)
	}
	outs := m["outbounds"].([]any)
	if outs[0].(map[string]any)["server_port"] != float64(2080) {
		t.Fatalf("socks outbound = %v", outs[0])
	}
	for _, o := range outs {
		if o.(map[string]any)["type"] == "dns" {
			t.Fatal("modern config must not have the dns-out outbound")
		}
	}
	if m["route"].(map[string]any)["auto_detect_interface"] != true {
		t.Fatal("auto_detect_interface missing")
	}
}

func TestTunConfigLegacy(t *testing.T) {
	conf, err := TunConfig(TunOptions{SocksAddr: "127.0.0.1:2080", ModernDNS: false})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(conf, &m)
	in := m["inbounds"].([]any)[0].(map[string]any)
	if in["sniff"] != true {
		t.Fatal("legacy config needs inbound sniff")
	}
	hasDNSOut := false
	for _, o := range m["outbounds"].([]any) {
		if o.(map[string]any)["type"] == "dns" {
			hasDNSOut = true
		}
	}
	if !hasDNSOut {
		t.Fatal("legacy config needs the dns-out outbound")
	}
	rule0 := m["route"].(map[string]any)["rules"].([]any)[0].(map[string]any)
	if rule0["outbound"] != "dns-out" {
		t.Fatalf("legacy dns rule = %v", rule0)
	}
}

func TestHTTPUpgradeTransport(t *testing.T) {
	p := Profile{
		ID: "hu1", Type: TVLESS, Host: "hu.example", Port: 2087,
		Fields: map[string]string{
			"uuid": "u-2", "security": "tls", "sni": "cdn.example",
			"transport": "httpupgrade", "path": "/upgrade", "ws_host": "cdn.example",
			"fingerprint": "chrome", "alpn": "http/1.1",
		},
	}
	// sing-box renderer
	sb, err := SingBoxConfig(p, SingBoxOptions{MixedPort: 2080})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(sb, &m)
	out := m["outbounds"].([]any)[0].(map[string]any)
	tr, ok := out["transport"].(map[string]any)
	if !ok || tr["type"] != "httpupgrade" || tr["path"] != "/upgrade" || tr["host"] != "cdn.example" {
		t.Fatalf("sing-box transport = %v", out["transport"])
	}
	// xray renderer
	xr, err := XrayConfig(p, XrayOptions{SocksPort: 2080})
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(xr, &m)
	out = m["outbounds"].([]any)[0].(map[string]any)
	ss := out["streamSettings"].(map[string]any)
	hu, ok := ss["httpupgradeSettings"].(map[string]any)
	if !ok || ss["network"] != "httpupgrade" || hu["path"] != "/upgrade" || hu["host"] != "cdn.example" {
		t.Fatalf("xray stream = %v", ss)
	}
}

func TestSemverPrefix(t *testing.T) {
	if m, mi, ok := parseSemverPrefix("sing-box version 1.11.5\n"); !ok || m != 1 || mi != 11 {
		t.Fatalf("1.11.5 -> %d.%d %v", m, mi, ok)
	}
	if m, mi, ok := parseSemverPrefix("sing-box version 1.12.0-alpha1"); !ok || m != 1 || mi != 12 {
		t.Fatalf("1.12.0 -> %d.%d %v", m, mi, ok)
	}
	if _, _, ok := parseSemverPrefix("garbage"); ok {
		t.Fatal("garbage must not parse")
	}
}
