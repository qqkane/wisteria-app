package core

// Renders an Xray-core JSON config (socks inbound + one outbound) for the
// protocols Xray handles: ss, vmess, vless, trojan. Used when the user
// picks xray-core as the multiprotocol engine — some servers behave better
// behind Xray than sing-box. Private-range bypass uses inline CIDRs so no
// geoip.dat/geosite.dat files are needed.

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// XraySupports reports whether the type can be served by Xray-core.
func XraySupports(t ProfileType) bool {
	switch t {
	case TSS, TVMess, TVLESS, TTrojan:
		return true
	}
	return false
}

// XrayOptions carries the settings that shape the generated config.
type XrayOptions struct {
	SocksPort     int
	Listen        string   // bind host for the socks inbound, "" = 127.0.0.1
	LogLevel      string   // app log level (trace..error), mapped to Xray's names
	BypassLAN     bool     // private ranges go direct
	RemoteDNS     string   // "" = Xray's default (system; UDP 53 via localhost)
	DirectDomains []string // user list: domains bypassing the tunnel
	DirectCIDRs   []string // user list: IPs/CIDRs bypassing the tunnel
	BlockDomains  []string // user list: blocked domains
	BlockCIDRs    []string // blocked IPs/CIDRs
}

// xrayLogLevel maps our settings level onto Xray's (trace/debug/info/
// warning/error; Xray has no trace and calls warn "warning").
func xrayLogLevel(l string) string {
	switch l {
	case "trace", "debug":
		return "debug"
	case "info":
		return "info"
	case "error":
		return "error"
	default: // "warn" and ""
		return "warning"
	}
}

func XrayConfig(p Profile, o XrayOptions) ([]byte, error) {
	if !XraySupports(p.Type) {
		return nil, fmt.Errorf("xray не поддерживает %q (нужен sing-box)", p.Type)
	}
	out, err := xrayOutbound(p)
	if err != nil {
		return nil, err
	}
	listen := firstNonEmpty(o.Listen, "127.0.0.1")
	cfg := map[string]any{
		"log": map[string]any{"loglevel": xrayLogLevel(o.LogLevel)},
		"inbounds": []any{map[string]any{
			"tag":      "socks-in",
			"listen":   listen,
			"port":     o.SocksPort,
			"protocol": "socks",
			"settings": map[string]any{"auth": "noauth", "udp": true},
			"sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls"}},
		}},
		"outbounds": []any{out, map[string]any{"tag": "direct", "protocol": "freedom"}},
	}
	var rules []any
	if o.BypassLAN {
		rules = append(rules, map[string]any{
			"type":        "field",
			"ip":          privateCIDRs,
			"outboundTag": "direct",
		})
	}
	if len(o.DirectDomains) > 0 {
		rules = append(rules, map[string]any{
			"type":        "field",
			"domain":      xrayDomains(o.DirectDomains),
			"outboundTag": "direct",
		})
	}
	if len(o.DirectCIDRs) > 0 {
		rules = append(rules, map[string]any{
			"type":        "field",
			"ip":          o.DirectCIDRs,
			"outboundTag": "direct",
		})
	}
	hasBlock := len(o.BlockDomains) > 0 || len(o.BlockCIDRs) > 0
	if len(o.BlockDomains) > 0 {
		rules = append(rules, map[string]any{
			"type":        "field",
			"domain":      xrayDomains(o.BlockDomains),
			"outboundTag": "block",
		})
	}
	if len(o.BlockCIDRs) > 0 {
		rules = append(rules, map[string]any{
			"type":        "field",
			"ip":          o.BlockCIDRs,
			"outboundTag": "block",
		})
	}
	if len(rules) > 0 {
		cfg["routing"] = map[string]any{"rules": rules}
	}
	if hasBlock {
		obs := cfg["outbounds"].([]any)
		cfg["outbounds"] = append(obs, map[string]any{"tag": "block", "protocol": "blackhole"})
	}
	if o.RemoteDNS != "" {
		cfg["dns"] = map[string]any{"servers": []string{o.RemoteDNS}}
	}
	return json.MarshalIndent(cfg, "", "  ")
}

// privateCIDRs: RFC1918/RFC4193/loopback/link-local, inline so the config
// never depends on geoip.dat.
var privateCIDRs = []string{
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
	"169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.168.0.0/16",
	"198.18.0.0/15", "224.0.0.0/3", "::1/128", "fc00::/7", "fe80::/10",
}

func xrayOutbound(p Profile) (map[string]any, error) {
	f := p.Fields
	stream := xrayStream(p, f)
	switch p.Type {
	case TSS:
		return map[string]any{
			"tag":      "proxy",
			"protocol": "shadowsocks",
			"settings": map[string]any{"servers": []any{map[string]any{
				"address": p.Host, "port": p.Port,
				"method": f["method"], "password": f["password"],
			}}},
		}, nil
	case TVMess:
		aid, _ := strconv.Atoi(firstNonEmpty(f["alter_id"], "0"))
		return map[string]any{
			"tag":      "proxy",
			"protocol": "vmess",
			"settings": map[string]any{"vnext": []any{map[string]any{
				"address": p.Host, "port": p.Port,
				"users": []any{map[string]any{
					"id": f["uuid"], "alterId": aid,
					"security": firstNonEmpty(f["security"], "auto"), "level": 0,
				}},
			}}},
			"streamSettings": stream,
		}, nil
	case TVLESS:
		u := map[string]any{"id": f["uuid"], "encryption": "none", "level": 0}
		if flow := f["flow"]; flow != "" {
			u["flow"] = flow
		}
		return map[string]any{
			"tag":      "proxy",
			"protocol": "vless",
			"settings": map[string]any{"vnext": []any{map[string]any{
				"address": p.Host, "port": p.Port, "users": []any{u},
			}}},
			"streamSettings": stream,
		}, nil
	case TTrojan:
		return map[string]any{
			"tag":      "proxy",
			"protocol": "trojan",
			"settings": map[string]any{"servers": []any{map[string]any{
				"address": p.Host, "port": p.Port, "password": f["password"], "level": 0,
			}}},
			"streamSettings": stream,
		}, nil
	}
	return nil, fmt.Errorf("неподдерживаемый тип %q", p.Type)
}

// xrayStream renders streamSettings (network + TLS/REALITY). Same field
// names as the sing-box renderer — both read Profile.Fields.
func xrayStream(p Profile, f map[string]string) map[string]any {
	network := firstNonEmpty(f["transport"], "tcp")
	security := ""
	switch {
	case f["security"] == "reality" || f["public_key"] != "":
		security = "reality"
	case f["security"] == "tls" || f["tls"] == "1" || p.Type == TTrojan:
		security = "tls"
	}
	s := map[string]any{"network": network}
	if security != "" {
		s["security"] = security
	}
	sni := firstNonEmpty(f["sni"], p.Host)
	switch security {
	case "tls":
		tls := map[string]any{"serverName": sni, "allowInsecure": f["insecure"] == "1" || f["allowInsecure"] == "1"}
		if fp := f["fingerprint"]; fp != "" {
			tls["fingerprint"] = fp
		}
		if alpn := f["alpn"]; alpn != "" {
			tls["alpn"] = splitCSV(alpn)
		}
		s["tlsSettings"] = tls
	case "reality":
		s["realitySettings"] = map[string]any{
			"serverName":  sni,
			"publicKey":   f["public_key"],
			"shortId":     f["short_id"],
			"fingerprint": firstNonEmpty(f["fingerprint"], "chrome"),
		}
	}
	switch network {
	case "ws":
		ws := map[string]any{}
		if pa := f["path"]; pa != "" {
			ws["path"] = pa
		}
		if h := f["ws_host"]; h != "" {
			ws["headers"] = map[string]any{"Host": h}
		}
		s["wsSettings"] = ws
	case "httpupgrade":
		hu := map[string]any{}
		if pa := f["path"]; pa != "" {
			hu["path"] = pa
		}
		if h := f["ws_host"]; h != "" {
			hu["host"] = h
		}
		s["httpupgradeSettings"] = hu
	case "grpc":
		s["grpcSettings"] = map[string]any{"serviceName": firstNonEmpty(f["service_name"], f["path"])}
	case "http", "h2":
		h := map[string]any{}
		if pa := f["path"]; pa != "" {
			h["path"] = pa
		}
		if hs := f["ws_host"]; hs != "" {
			h["host"] = []string{hs}
		}
		s["httpSettings"] = h
	}
	return s
}

// xrayDomains renders plain domains as Xray's "domain:" pattern (matches the
// domain and its subdomains); entries that already carry an Xray prefix
// ("geosite:", "regexp:", …) pass through untouched.
func xrayDomains(list []string) []string {
	out := make([]string, 0, len(list))
	for _, d := range list {
		if strings.Contains(d, ":") {
			out = append(out, d)
		} else {
			out = append(out, "domain:"+d)
		}
	}
	return out
}

func splitCSV(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
