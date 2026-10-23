package core

// Renders a sing-box JSON config (one mixed inbound + one outbound) for the
// sing-box protocol family. Kept deliberately minimal and stable: plain
// tcp/ws/grpc/http transports, TLS and REALITY — the surface real share
// links actually use.

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// SingBoxOptions carries the settings that shape the generated config.
type SingBoxOptions struct {
	MixedPort     int
	Listen        string   // bind host for the mixed inbound, "" = 127.0.0.1
	LogLevel      string   // sing-box log level, "" = warn
	BypassLAN     bool     // private ranges go direct
	RemoteDNS     string   // resolver for the outbound server name, "" = system
	DirectDomains []string // user list: domains bypassing the tunnel
	DirectCIDRs   []string // user list: IPs/CIDRs bypassing the tunnel
	BlockDomains  []string // user list: blocked domains
	BlockCIDRs    []string // blocked IPs/CIDRs
}

func SingBoxConfig(p Profile, o SingBoxOptions) ([]byte, error) {
	if !SingBox(p.Type) {
		return nil, fmt.Errorf("профиль %q не относится к sing-box протоколам", p.ID)
	}
	out, err := singBoxOutbound(p)
	if err != nil {
		return nil, err
	}
	level := o.LogLevel
	if level == "" {
		level = "warn"
	}
	listen := o.Listen
	if listen == "" {
		listen = "127.0.0.1"
	}
	route := map[string]any{"final": "proxy"}
	var rules []any
	if o.BypassLAN {
		rules = append(rules, map[string]any{"ip_is_private": true, "outbound": "direct"})
	}
	if len(o.DirectDomains) > 0 {
		rules = append(rules, map[string]any{"domain_suffix": o.DirectDomains, "outbound": "direct"})
	}
	if len(o.DirectCIDRs) > 0 {
		rules = append(rules, map[string]any{"ip_cidr": o.DirectCIDRs, "outbound": "direct"})
	}
	hasBlock := len(o.BlockDomains) > 0 || len(o.BlockCIDRs) > 0
	if len(o.BlockDomains) > 0 {
		rules = append(rules, map[string]any{"domain_suffix": o.BlockDomains, "outbound": "block"})
	}
	if len(o.BlockCIDRs) > 0 {
		rules = append(rules, map[string]any{"ip_cidr": o.BlockCIDRs, "outbound": "block"})
	}
	if len(rules) > 0 {
		route["rules"] = rules
	}
	outbounds := []any{out, map[string]any{"type": "direct", "tag": "direct"}}
	if hasBlock {
		outbounds = append(outbounds, map[string]any{"type": "block", "tag": "block"})
	}
	cfg := map[string]any{
		"log": map[string]any{"level": level, "timestamp": true},
		"inbounds": []any{map[string]any{
			"type":        "mixed",
			"tag":         "in",
			"listen":      listen,
			"listen_port": o.MixedPort,
		}},
		"outbounds": outbounds,
		"route":     route,
	}
	if o.RemoteDNS != "" {
		// Resolve the outbound SERVER's own domain through the chosen DNS.
		cfg["dns"] = map[string]any{
			"servers": []any{map[string]any{"tag": "remote", "address": o.RemoteDNS}},
		}
		route["default_domain_resolver"] = "remote"
	}
	return json.MarshalIndent(cfg, "", "  ")
}

type sob map[string]any

func singBoxOutbound(p Profile) (sob, error) {
	f := p.Fields
	o := sob{"tag": "proxy", "server": p.Host, "server_port": p.Port}
	switch p.Type {
	case TSS:
		o["type"] = "shadowsocks"
		o["method"] = f["method"]
		o["password"] = f["password"]
	case TVMess:
		o["type"] = "vmess"
		o["uuid"] = f["uuid"]
		o["security"] = firstNonEmpty(f["security"], "auto")
		aid, _ := strconv.Atoi(firstNonEmpty(f["alter_id"], "0"))
		o["alter_id"] = aid
		applyTLS(o, f, false)
		if f["tls"] == "1" {
			markTLS(o, f)
		}
		applyTransport(o, f)
	case TVLESS:
		o["type"] = "vless"
		o["uuid"] = f["uuid"]
		if flow := f["flow"]; flow != "" {
			o["flow"] = flow
		}
		applyTLS(o, f, false)
		applyTransport(o, f)
	case TTrojan:
		o["type"] = "trojan"
		o["password"] = f["password"]
		applyTLS(o, f, true) // trojan is always TLS
		applyTransport(o, f)
	case THysteria2:
		o["type"] = "hysteria2"
		o["password"] = f["password"]
		if f["obfs"] != "" {
			o["obfs"] = sob{"type": f["obfs"], "password": f["obfs_password"]}
		}
		applyTLS(o, f, true) // hysteria2 is always TLS
	case TTuic:
		o["type"] = "tuic"
		o["uuid"] = f["uuid"]
		o["password"] = f["password"]
		o["congestion_control"] = firstNonEmpty(f["congestion_control"], "bbr")
		o["udp_relay_mode"] = firstNonEmpty(f["udp_relay_mode"], "native")
		applyTLS(o, f, true) // tuic is always TLS
	default:
		return nil, fmt.Errorf("неподдерживаемый тип %q", p.Type)
	}
	if strings.TrimSpace(strconv.Quote(fmt.Sprint(o["server"]))) == `""` {
		return nil, fmt.Errorf("пустой адрес сервера")
	}
	return o, nil
}

// applyTLS adds the tls block when the link says TLS (security=tls/reality,
// REALITY params present). force marks protocols that are always TLS
// (trojan, hysteria2, tuic) even when the link carries no explicit marker.
func applyTLS(o sob, f map[string]string, force bool) {
	sec := f["security"]
	tlsOn := force || sec == "tls" || sec == "reality" || f["public_key"] != ""
	if !tlsOn {
		return
	}
	markTLS(o, f)
	if sec == "reality" || f["public_key"] != "" {
		t := o["tls"].(sob)
		t["reality"] = sob{
			"enabled":    true,
			"public_key": f["public_key"],
			"short_id":   f["short_id"],
		}
	}
}

// markTLS installs the base tls block (server_name/alpn/insecure/utls).
func markTLS(o sob, f map[string]string) {
	t, ok := o["tls"].(sob)
	if !ok {
		t = sob{"enabled": true}
	}
	if sni := f["sni"]; sni != "" && t["server_name"] == nil {
		t["server_name"] = sni
	}
	if alpn := f["alpn"]; alpn != "" && t["alpn"] == nil {
		t["alpn"] = strings.Split(alpn, ",")
	}
	if f["insecure"] == "1" || f["allowInsecure"] == "1" {
		t["insecure"] = true
	}
	if fp := f["fingerprint"]; fp != "" && t["utls"] == nil {
		t["utls"] = sob{"enabled": true, "fingerprint": fp}
	}
	o["tls"] = t
}

func applyTransport(o sob, f map[string]string) {
	switch f["transport"] {
	case "ws":
		tr := sob{"type": "ws"}
		if p := f["path"]; p != "" {
			tr["path"] = p
		}
		if h := f["ws_host"]; h != "" {
			tr["headers"] = sob{"Host": h}
		}
		o["transport"] = tr
	case "httpupgrade":
		tr := sob{"type": "httpupgrade"}
		if p := f["path"]; p != "" {
			tr["path"] = p
		}
		if h := f["ws_host"]; h != "" {
			tr["host"] = h
		}
		o["transport"] = tr
	case "grpc":
		tr := sob{"type": "grpc"}
		if sn := f["service_name"]; sn != "" {
			tr["service_name"] = sn
		}
		o["transport"] = tr
	case "http", "h2":
		tr := sob{"type": "http"}
		if p := f["path"]; p != "" {
			tr["path"] = p
		}
		if h := f["ws_host"]; h != "" {
			tr["host"] = []string{h}
		}
		o["transport"] = tr
	}
}
