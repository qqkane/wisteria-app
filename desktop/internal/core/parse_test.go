package core

import (
	"strings"
	"testing"
)

func TestParseSS_SIP002(t *testing.T) {
	// aes-256-gcm:secretpass base64 (std, padded)
	p, err := ParseLink("ss://YWVzLTI1Ni1nY206c2VjcmV0cGFzcw==@1.2.3.4:8388#My%20SS")
	if err != nil {
		t.Fatal(err)
	}
	if p.Type != TSS || p.Host != "1.2.3.4" || p.Port != 8388 {
		t.Fatalf("base fields: %+v", p)
	}
	if p.Fields["method"] != "aes-256-gcm" || p.Fields["password"] != "secretpass" {
		t.Fatalf("fields: %v", p.Fields)
	}
	if p.Name != "My SS" {
		t.Fatalf("name = %q", p.Name)
	}
}

func TestParseSS_LegacyWholeBase64(t *testing.T) {
	// legacy: base64(method:password@host:port)
	p, err := ParseLink("ss://YWVzLTI1Ni1nY206cHdAaG9zdC5leGFtcGxlOjQ0Mw==#legacy")
	if err != nil {
		t.Fatal(err)
	}
	if p.Host != "host.example" || p.Port != 443 || p.Fields["method"] != "aes-256-gcm" || p.Fields["password"] != "pw" {
		t.Fatalf("%+v fields=%v", p, p.Fields)
	}
}

func TestParseSS_PluginRejected(t *testing.T) {
	if _, err := ParseLink("ss://YWVzLTI1Ni1nY206cHc=@1.2.3.4:8388?plugin=obfs-local#x"); err == nil {
		t.Fatal("plugin links must be rejected, not silently misconfigured")
	}
}

func TestParseVLESS_RealityWS(t *testing.T) {
	p, err := ParseLink("vless://b831381d-6324-4d53-ad4f-8cda48b30811@example.com:443?encryption=none&security=reality&sni=www.microsoft.com&fp=chrome&pbk=SbVKOEMjK0sIlbwg4akyBg5mL5KZwwB-ed4eEE7YnRc&sid=6ba85179&type=ws&path=%2Fws&flow=xtls-rprx-vision#RealityNode")
	if err != nil {
		t.Fatal(err)
	}
	if p.Type != TVLESS || p.Host != "example.com" || p.Port != 443 {
		t.Fatalf("%+v", p)
	}
	f := p.Fields
	if f["uuid"] != "b831381d-6324-4d53-ad4f-8cda48b30811" || f["security"] != "reality" ||
		f["sni"] != "www.microsoft.com" || f["public_key"] != "SbVKOEMjK0sIlbwg4akyBg5mL5KZwwB-ed4eEE7YnRc" ||
		f["short_id"] != "6ba85179" || f["transport"] != "ws" || f["path"] != "/ws" ||
		f["flow"] != "xtls-rprx-vision" {
		t.Fatalf("fields: %v", f)
	}
	if p.Name != "RealityNode" {
		t.Fatalf("name %q", p.Name)
	}
}

func TestParseTrojan(t *testing.T) {
	p, err := ParseLink("trojan://pass%40word@t.example:443?sni=real.example&type=ws&path=%2Ftr#trojan")
	if err != nil {
		t.Fatal(err)
	}
	if p.Type != TTrojan || p.Fields["password"] != "pass@word" || p.Fields["sni"] != "real.example" ||
		p.Fields["transport"] != "ws" || p.Fields["path"] != "/tr" {
		t.Fatalf("%+v %v", p, p.Fields)
	}
}

func TestParseHysteria2(t *testing.T) {
	p, err := ParseLink("hy2://authpass@h.example:8443?sni=h2.example&insecure=1&obfs=salamander&obfs-password=obfsp#hy2node")
	if err != nil {
		t.Fatal(err)
	}
	if p.Type != THysteria2 || p.Fields["password"] != "authpass" || p.Fields["insecure"] != "1" ||
		p.Fields["obfs"] != "salamander" || p.Fields["obfs_password"] != "obfsp" {
		t.Fatalf("%+v %v", p, p.Fields)
	}
}

func TestParseTuic(t *testing.T) {
	p, err := ParseLink("tuic://uuid-1:secretpw@t.example:443?congestion_control=bbr&udp_relay_mode=native&sni=t.example#tuic")
	if err != nil {
		t.Fatal(err)
	}
	if p.Type != TTuic || p.Fields["uuid"] != "uuid-1" || p.Fields["password"] != "secretpw" ||
		p.Fields["congestion_control"] != "bbr" {
		t.Fatalf("%+v %v", p, p.Fields)
	}
}

func TestParseVMess(t *testing.T) {
	// {"v":"2","ps":"vm1","add":"v.example","port":"443","id":"<uuid>","aid":"0",
	//  "scy":"auto","net":"ws","host":"cdn.example","path":"/vm","tls":"tls"}
	uri := "vmess://eyJ2IjoiMiIsInBzIjoidm0xIiwiYWRkIjoidi5leGFtcGxlIiwicG9ydCI6IjQ0MyIsImlkIjoiYjgzMTM4MWQtNjMyNC00ZDUzLWFkNGYtOGNkYTQ4YjMwODEyIiwiYWlkIjoiMCIsInNjeSI6ImF1dG8iLCJuZXQiOiJ3cyIsImhvc3QiOiJjZG4uZXhhbXBsZSIsInBhdGgiOiIvdm0iLCJ0bHMiOiJ0bHMifQ=="
	p, err := ParseLink(uri)
	if err != nil {
		t.Fatal(err)
	}
	if p.Type != TVMess || p.Host != "v.example" || p.Port != 443 || p.Name != "vm1" {
		t.Fatalf("%+v", p)
	}
	f := p.Fields
	if f["uuid"] != "b831381d-6324-4d53-ad4f-8cda48b30812" || f["transport"] != "ws" ||
		f["path"] != "/vm" || f["ws_host"] != "cdn.example" || f["tls"] != "1" || f["sni"] != "cdn.example" {
		t.Fatalf("fields: %v", f)
	}
}

func TestParseWisteriaLink(t *testing.T) {
	pub := "q5LinLVGCGUA8WzbRTjylGRv36jrCIb+V7tH8Y5cXVo="
	// base64url of the pub (no padding): recompute by hand here.
	b64url := strings.NewReplacer("+", "-", "/", "_", "=", "").Replace(pub)
	uri := "wisteria://" + b64url + "@w.example:47823?pad=64-1448&obf_mu=6.7&obf_sigma=0.6&jitter_us=1&coalesce_ms=5&mimic=1&filler=80-400&filler_ratio=5#w-node"
	p, err := ParseLink(uri)
	if err != nil {
		t.Fatal(err)
	}
	if p.Type != TWisteria || p.Host != "w.example" || p.Port != 47823 || p.Name != "w-node" {
		t.Fatalf("%+v", p)
	}
	f := p.Fields
	if f["pub"] != pub || f["min_pad"] != "64" || f["max_pad"] != "1448" ||
		f["obf_mu"] != "6.7" || f["obf_sigma"] != "0.6" || f["jitter_us"] != "1" ||
		f["coalesce_ms"] != "5" || f["mimic"] != "1" || f["filler_min"] != "80" ||
		f["filler_max"] != "400" || f["filler_ratio"] != "5" {
		t.Fatalf("fields: %v", f)
	}
}

func TestParseWisteriaTOMLImport(t *testing.T) {
	tomlText := `
[server]
endpoint = "srv.example:47823"

[crypto]
static_private = "CL1ENTPRIVATEKEYBASE64=="
server_static_public = "q5LinLVGCGUA8WzbRTjylGRv36jrCIb+V7tH8Y5cXVo="

[obfuscation]
mu_log  = 6.6846
sigma   = 0.6
min_pad = 64
max_pad = 1448
jitter_micro_ms = 1
coalesce_ms = 5
mimic_tls = 1
filler_min   = 80
filler_max   = 400
filler_ratio = 5
rekey_frames = 16777216
`
	p, err := ParseWisteriaTOML(tomlText)
	if err != nil {
		t.Fatal(err)
	}
	if p.Host != "srv.example" || p.Port != 47823 {
		t.Fatalf("%+v", p)
	}
	if p.ClientPriv != "CL1ENTPRIVATEKEYBASE64==" {
		t.Fatalf("client key lost: %q", p.ClientPriv)
	}
	if p.Fields["pub"] == "" || p.Fields["min_pad"] != "64" || p.Fields["rekey"] != "16777216" {
		t.Fatalf("fields: %v", p.Fields)
	}
}

func TestParseGarbage(t *testing.T) {
	for _, bad := range []string{
		"", "http://example.com", "ss://!!!@bad", "vless://@host:1",
		"ftp://x@1.2.3.4:5",
	} {
		if _, err := ParseLink(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestSplitHostPortIPv6(t *testing.T) {
	h, p, err := splitHostPort("[2001:db8::1]:443")
	if err != nil || h != "2001:db8::1" || p != 443 {
		t.Fatalf("%q %d %v", h, p, err)
	}
	if got := joinHostPort("2001:db8::1", 443); got != "[2001:db8::1]:443" {
		t.Fatalf("joinHostPort = %q", got)
	}
}
