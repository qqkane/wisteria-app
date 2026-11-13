package core

// TunConfig renders the sing-box helper config for TUN mode: a tun inbound
// with auto_route captures all system traffic and forwards it into the main
// core's SOCKS inbound. This works for every protocol — including the
// native wysteria core, which has no TUN of its own. The server's own
// addresses are excluded from the tunnel, otherwise the main core's
// upstream connection would loop back into the TUN.
//
// Both address families are assigned to the adapter (a ULA for IPv6); with
// auto_route sing-box installs a default route per configured family, so
// IPv6 traffic is captured too instead of leaking around the tunnel.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
)

// tunInterfaceName is the tun adapter created by the helper (wintun on
// Windows, a plain tun device on Linux — both named wisteria0).
const tunInterfaceName = "wisteria0"

type TunOptions struct {
	SocksAddr   string   // the main core's socks inbound, host:port
	ServerIPs   []string // resolved server addresses, routed direct
	BypassLAN   bool
	RemoteDNS   string // resolver for hijacked DNS, "" = tcp://8.8.8.8 via tunnel
	ModernDNS   bool   // sing-box >= 1.11: route rules use "action" (dns-out outbound was removed in 1.12)
	LogLevel    string
	Stack       string // "mixed" (default) | "gvisor" | "system"
	StrictRoute bool   // tighter leak protection; may break LAN access
}

func TunConfig(o TunOptions) ([]byte, error) {
	host, portStr, err := net.SplitHostPort(o.SocksAddr)
	if err != nil {
		return nil, fmt.Errorf("socks-адрес %q: %v", o.SocksAddr, err)
	}
	port, err := net.LookupPort("tcp", portStr)
	if err != nil {
		return nil, fmt.Errorf("socks-порт %q: %v", portStr, err)
	}
	level := firstNonEmpty(o.LogLevel, "warn")

	stack := o.Stack
	if stack != "gvisor" && stack != "system" {
		stack = "mixed"
	}
	in := map[string]any{
		"type":           "tun",
		"tag":            "tun-in",
		"interface_name": tunInterfaceName,
		// sing-box 1.10+ merged inet4_address/inet6_address into this array;
		// auto_route turns each configured family into its default route
		// (0.0.0.0/0 and ::/0) pointed at the adapter.
		"address":      []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"},
		"mtu":          9000,
		"auto_route":   true,
		"strict_route": o.StrictRoute,
		"stack":        stack,
	}
	// Legacy sing-box needs inbound sniffing for the dns protocol rule;
	// modern ones sniff via the route rule action below.
	if !o.ModernDNS {
		in["sniff"] = true
	}

	var rules []any
	if o.ModernDNS {
		rules = append(rules, map[string]any{
			"inbound": []string{"tun-in"}, "protocol": "dns", "action": "hijack-dns",
		})
	} else {
		rules = append(rules, map[string]any{
			"inbound": []string{"tun-in"}, "protocol": "dns", "outbound": "dns-out",
		})
	}
	if len(o.ServerIPs) > 0 {
		rules = append(rules, map[string]any{"ip_cidr": o.ServerIPs, "outbound": "direct"})
	}
	if o.BypassLAN {
		rules = append(rules, map[string]any{"ip_is_private": true, "outbound": "direct"})
	}

	outbounds := []any{
		map[string]any{"type": "socks", "tag": "proxy", "server": host, "server_port": port},
		map[string]any{"type": "direct", "tag": "direct"},
	}
	if !o.ModernDNS {
		outbounds = append(outbounds, map[string]any{"type": "dns", "tag": "dns-out"})
	}

	cfg := map[string]any{
		"log":       map[string]any{"level": level, "timestamp": true},
		"inbounds":  []any{in},
		"outbounds": outbounds,
		"route": map[string]any{
			"rules":                 rules,
			"final":                 "proxy",
			"auto_detect_interface": true,
		},
		"dns": map[string]any{"servers": []any{map[string]any{
			"tag": "remote", "address": firstNonEmpty(o.RemoteDNS, "tcp://8.8.8.8"), "detour": "proxy",
		}}},
	}
	return json.MarshalIndent(cfg, "", "  ")
}

// resolveServerAddrs turns the profile host into tunnel-excluded IPs. IPv4
// first (the usual case); v6 only if no v4 was returned.
func resolveServerAddrs(host string) ([]string, error) {
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	var v4, v6 []string
	for _, ip := range ips {
		if t := ip.To4(); t != nil {
			v4 = append(v4, t.String())
		} else {
			v6 = append(v6, ip.String())
		}
	}
	out := append(v4, v6...)
	if len(out) == 0 {
		return nil, errors.New("адреса не получены")
	}
	return out, nil
}

// singBoxModernDNS runs `sing-box version` and reports whether route rules
// with "action" are available (1.11+). On any doubt assume modern: current
// releases reject the removed dns-out outbound.
func singBoxModernDNS(bin string) bool {
	out, err := runCoreOutput(bin, "version")
	if err != nil {
		return true
	}
	major, minor, ok := parseSemverPrefix(string(out))
	if !ok {
		return true
	}
	return major > 1 || (major == 1 && minor >= 11)
}

// parseSemverPrefix finds the first X.Y[.Z] token in s.
func parseSemverPrefix(s string) (major, minor int, ok bool) {
	num := func(t string) (int, bool) {
		n := 0
		if t == "" {
			return 0, false
		}
		for _, r := range t {
			if r < '0' || r > '9' {
				return 0, false
			}
			n = n*10 + int(r-'0')
		}
		return n, true
	}
	for _, field := range fieldsOf(s) {
		for i := 0; i < len(field); i++ {
			if field[i] >= '0' && field[i] <= '9' {
				parts := splitDots(field[i:])
				if len(parts) < 2 {
					break
				}
				m, ok1 := num(parts[0])
				mi, ok2 := num(parts[1])
				if ok1 && ok2 {
					return m, mi, true
				}
				break
			}
		}
	}
	return 0, 0, false
}

func fieldsOf(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
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

func splitDots(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '.' || r == '-' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	out = append(out, cur)
	return out
}
