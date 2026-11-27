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
	// Modern sing-box sniffs through a route action: without it the
	// "protocol": "dns" condition never matches (the inbound sniff option
	// is gone in 1.11+).
	if rules[0].(map[string]any)["action"] != "sniff" {
		t.Fatalf("first rule = %v, want the sniff action", rules[0])
	}
	if rules[1].(map[string]any)["action"] != "hijack-dns" {
		t.Fatalf("dns rule = %v", rules[1])
	}
	dns53 := rules[2].(map[string]any)
	if dns53["action"] != "hijack-dns" || dns53["port"] != float64(53) {
		t.Fatalf("port-53 fallback rule = %v", dns53)
	}
	loop := rules[3].(map[string]any)
	if loop["outbound"] != "direct" {
		t.Fatalf("loopback rule = %v", loop)
	}
	excl := rules[4].(map[string]any)
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
	// 1.11 and older still accept the legacy address schema; emitting the
	// typed one there would be just as fatal as the reverse.
	srv := m["dns"].(map[string]any)["servers"].([]any)[0].(map[string]any)
	if srv["address"] != "tcp://8.8.8.8" || srv["detour"] != "proxy" {
		t.Fatalf("legacy dns server = %v", srv)
	}
	if _, has := srv["type"]; has {
		t.Fatalf("legacy dns server must not use the typed schema: %v", srv)
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

// ---- sing-box 1.12+ TUN generation (phase 4) ----

func tunCfgMap(t *testing.T, o TunOptions) map[string]any {
	t.Helper()
	conf, err := TunConfig(o)
	if err != nil {
		t.Fatalf("TunConfig: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(conf, &m); err != nil {
		t.Fatalf("json: %v", err)
	}
	return m
}

func tunRules(m map[string]any) []any {
	return m["route"].(map[string]any)["rules"].([]any)
}

func ruleWith(rules []any, key string) map[string]any {
	for _, r := range rules {
		rm := r.(map[string]any)
		if _, ok := rm[key]; ok {
			return rm
		}
	}
	return nil
}

func tunDNSServers(t *testing.T, m map[string]any) []any {
	dns, ok := m["dns"].(map[string]any)
	if !ok {
		t.Fatalf("dns section missing: %v", m["dns"])
	}
	return dns["servers"].([]any)
}

func dnsServerByTag(t *testing.T, m map[string]any, tag string) map[string]any {
	for _, s := range tunDNSServers(t, m) {
		sm := s.(map[string]any)
		if sm["tag"] == tag {
			return sm
		}
	}
	return nil
}

func TestTunConfigModern112Plus(t *testing.T) {
	yes := true
	m := tunCfgMap(t, TunOptions{
		SocksAddr:       "127.0.0.1:2080",
		ServerIPs:       []string{"203.0.113.7", "2001:db8::1", "203.0.113.7"},
		CoreProcessPath: `C:\app\bin\wisteria-client.exe`,
		ModernDNS:       true,
		NewDNSFormat:    &yes,
	})

	// Sniffing must be a route action BEFORE the dns match: a modern
	// config has no inbound sniff option, so without this rule the
	// protocol condition never fires.
	rules := tunRules(m)
	if rules[0].(map[string]any)["action"] != "sniff" {
		t.Fatalf("rules[0] = %v, want the sniff action", rules[0])
	}
	sniffAt, hijackAt := -1, -1
	for i, r := range rules {
		rm := r.(map[string]any)
		if rm["action"] == "sniff" {
			sniffAt = i
		}
		if rm["action"] == "hijack-dns" && rm["protocol"] == "dns" && hijackAt < 0 {
			hijackAt = i
		}
	}
	if sniffAt < 0 || hijackAt < 0 || sniffAt > hijackAt {
		t.Fatalf("sniff (%d) must precede hijack-dns (%d): %v", sniffAt, hijackAt, rules)
	}
	if ruleWith(rules, "port") == nil {
		t.Fatalf("missing the port-53 hijack fallback: %v", rules)
	}

	// Loopback and the VPN server leave the tunnel directly.
	loop := ruleWith(rules, "ip_cidr")
	if loop == nil || loop["outbound"] != "direct" {
		t.Fatalf("loopback rule = %v", loop)
	}
	loopCIDRs := loop["ip_cidr"].([]any)
	if len(loopCIDRs) != 2 || loopCIDRs[0] != "127.0.0.0/8" || loopCIDRs[1] != "::1/128" {
		t.Fatalf("loopback cidrs = %v", loopCIDRs)
	}
	proc := ruleWith(rules, "process_path")
	if proc == nil || proc["outbound"] != "direct" {
		t.Fatalf("process_path rule = %v", proc)
	}
	if got := proc["process_path"].([]any)[0]; got != `C:\app\bin\wisteria-client.exe` {
		t.Fatalf("process_path = %v", got)
	}

	// Bare addresses become host prefixes, deduplicated, and the kernel
	// excludes them from the adapter entirely.
	in := m["inbounds"].([]any)[0].(map[string]any)
	excl, ok := in["route_exclude_address"].([]any)
	if !ok {
		t.Fatalf("route_exclude_address missing: %v", in)
	}
	if len(excl) != 2 || excl[0] != "203.0.113.7/32" || excl[1] != "2001:db8::1/128" {
		t.Fatalf("route_exclude_address = %v", excl)
	}
	var serverRule map[string]any
	for _, r := range rules {
		rm := r.(map[string]any)
		if c, ok := rm["ip_cidr"].([]any); ok && len(c) == 2 && c[0] == "203.0.113.7/32" {
			serverRule = rm
		}
	}
	if serverRule == nil || serverRule["outbound"] != "direct" {
		t.Fatalf("server direct rule = %v", serverRule)
	}

	// Typed DNS server on 1.12+, plus the local one the engine requires.
	remote := dnsServerByTag(t, m, "remote")
	if remote == nil {
		t.Fatal("remote DNS server missing")
	}
	if remote["type"] != "tcp" || remote["server"] != "8.8.8.8" || remote["detour"] != "proxy" {
		t.Fatalf("remote DNS = %v", remote)
	}
	if _, has := remote["address"]; has {
		t.Fatalf("legacy address key in a 1.12+ config: %v", remote)
	}
	local := dnsServerByTag(t, m, "local")
	if local == nil || local["type"] != "local" {
		t.Fatalf("local DNS server = %v", local)
	}
	if got := m["route"].(map[string]any)["default_domain_resolver"]; got != "local" {
		t.Fatalf("default_domain_resolver = %v, want local", got)
	}
	if got := m["dns"].(map[string]any)["final"]; got != "remote" {
		t.Fatalf("dns.final = %v, want remote", got)
	}
}

func TestTunConfigUpstreamTCPOnly(t *testing.T) {
	cases := []struct{ in, wantType, wantServer string }{
		{"1.1.1.1", "tcp", "1.1.1.1"},
		{"udp://8.8.8.8", "tcp", "8.8.8.8"},
		{"", "tcp", "8.8.8.8"},
		// tls:// already runs over TCP and is left as the user wrote it.
		{"tls://dns.quad9.net", "tls", "dns.quad9.net"},
	}
	for _, tc := range cases {
		m := tunCfgMap(t, TunOptions{
			SocksAddr:       "127.0.0.1:2080",
			UpstreamTCPOnly: true,
			RemoteDNS:       tc.in,
			ModernDNS:       true,
		})
		remote := dnsServerByTag(t, m, "remote")
		if remote["type"] != tc.wantType || remote["server"] != tc.wantServer {
			t.Fatalf("RemoteDNS %q -> %v, want %s/%s", tc.in, remote, tc.wantType, tc.wantServer)
		}
		proxy := m["outbounds"].([]any)[0].(map[string]any)
		if proxy["type"] != "socks" || proxy["network"] != "tcp" {
			t.Fatalf("socks outbound = %v, want network=tcp", proxy)
		}
		// UDP reject must come after the DNS hijack: hijacked DNS is
		// terminal, all remaining UDP (QUIC) is rejected.
		rules := tunRules(m)
		hijackAt, rejectAt := -1, -1
		for i, r := range rules {
			rm := r.(map[string]any)
			if rm["action"] == "hijack-dns" && hijackAt < 0 {
				hijackAt = i
			}
			if rm["network"] == "udp" && rm["action"] == "reject" {
				rejectAt = i
			}
		}
		if hijackAt < 0 || rejectAt < 0 || rejectAt < hijackAt {
			t.Fatalf("RemoteDNS %q: rules = %v, want hijack-dns before the UDP reject", tc.in, rules)
		}
		// No legacy block outbound in a modern config.
		for _, o := range m["outbounds"].([]any) {
			if o.(map[string]any)["type"] == "block" {
				t.Fatalf("modern config must not emit the removed block outbound: %v", o)
			}
		}
	}
}

func TestBuildSingBoxDNSServer(t *testing.T) {
	srv, needsLocal := buildSingBoxDNSServer("d", "tcp://8.8.8.8", "proxy", true)
	if srv["type"] != "tcp" || srv["server"] != "8.8.8.8" || srv["detour"] != "proxy" {
		t.Fatalf("tcp server = %v", srv)
	}
	if needsLocal {
		t.Fatal("an IP literal needs no local resolver")
	}
	if _, has := srv["server_port"]; has {
		t.Fatalf("no port was given: %v", srv)
	}

	srv, _ = buildSingBoxDNSServer("d", "udp://1.1.1.1:5353", "", true)
	if srv["type"] != "udp" || srv["server_port"] != 5353 {
		t.Fatalf("udp server = %v", srv)
	}

	srv, needsLocal = buildSingBoxDNSServer("d", "tls://dns.quad9.net:853", "", true)
	if srv["type"] != "tls" || srv["server"] != "dns.quad9.net" || srv["server_port"] != 853 {
		t.Fatalf("tls server = %v", srv)
	}
	if srv["domain_resolver"] != "local" || !needsLocal {
		t.Fatalf("a hostname needs domain_resolver=local: %v (needsLocal=%v)", srv, needsLocal)
	}

	srv, needsLocal = buildSingBoxDNSServer("d", "https://dns.google/dns-query", "", true)
	if srv["type"] != "https" || srv["path"] != "/dns-query" || srv["domain_resolver"] != "local" || !needsLocal {
		t.Fatalf("DoH server = %v (needsLocal=%v)", srv, needsLocal)
	}

	srv, _ = buildSingBoxDNSServer("d", "https://1.1.1.1:443", "", true)
	if srv["type"] != "https" || srv["server"] != "1.1.1.1" || srv["server_port"] != 443 {
		t.Fatalf("DoH with explicit port = %v", srv)
	}
	if _, has := srv["path"]; has {
		t.Fatalf("an empty DoH path must stay implicit: %v", srv)
	}

	srv, _ = buildSingBoxDNSServer("d", "8.8.8.8", "", true)
	if srv["type"] != "udp" || srv["server"] != "8.8.8.8" {
		t.Fatalf("bare IP = %v", srv)
	}

	srv, _ = buildSingBoxDNSServer("d", "[2606:4700:4700::1111]:53", "", true)
	if srv["type"] != "udp" || srv["server"] != "2606:4700:4700::1111" || srv["server_port"] != 53 {
		t.Fatalf("IPv6 server = %v", srv)
	}

	srv, needsLocal = buildSingBoxDNSServer("d", "local", "", true)
	if srv["type"] != "local" || srv["tag"] != "d" || needsLocal {
		t.Fatalf("local server = %v", srv)
	}
	if _, has := srv["server"]; has {
		t.Fatalf("local must not carry a server field: %v", srv)
	}

	legacy, _ := buildSingBoxDNSServer("d", "tcp://8.8.8.8", "proxy", false)
	if legacy["address"] != "tcp://8.8.8.8" || legacy["detour"] != "proxy" {
		t.Fatalf("legacy server = %v", legacy)
	}
	if _, has := legacy["type"]; has {
		t.Fatalf("legacy server must not use the typed schema: %v", legacy)
	}
}
