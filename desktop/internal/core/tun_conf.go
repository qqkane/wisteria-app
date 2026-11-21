package core

// TunConfig renders the sing-box helper config for TUN mode: a tun inbound
// with auto_route captures all system traffic and forwards it into the main
// core's SOCKS inbound. This works for every protocol — including the
// native wysteria core, which has no TUN of its own. The server's own
// addresses are excluded from the tunnel, otherwise the main core's
// upstream connection would loop back into the TUN.
//
// Two sing-box generations are supported. 1.11 replaced the dns-out/block
// outbounds and the inbound sniff option with route actions; 1.12 replaced
// the legacy {"address": ...} DNS server schema with typed servers and
// started requiring route.default_domain_resolver. 1.14 removed the legacy
// DNS schema entirely, so a config written for the old layout makes a fresh
// sing-box.exe exit on startup. SingBoxCaps carries the probed generation
// into both renderers.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// tunInterfaceName is the tun adapter created by the helper (wintun on
// Windows, a plain tun device on Linux — both named wisteria0).
const tunInterfaceName = "wisteria0"

// SingBoxCaps is the capability set of one sing-box binary, derived from
// its `version` output. Both flags exist because 1.11 and 1.12 each removed
// a piece of the config the app used to emit.
type SingBoxCaps struct {
	Major         int
	Minor         int
	ModernActions bool // sing-box >= 1.11: route actions (sniff, hijack-dns, reject); no dns-out / block outbounds; no inbound sniff
	NewDNSFormat  bool // sing-box >= 1.12: typed DNS servers ({type, server, ...}) instead of removed {address}; requires route.default_domain_resolver
}

type TunOptions struct {
	SocksAddr       string   // the main core's socks inbound, host:port
	ServerIPs       []string // resolved server addresses, routed direct
	CoreProcessPath string   // absolute path to the main core executable (bypass TUN to prevent self-routing loops)
	UpstreamTCPOnly bool     // true for wisteria-client (armon/go-socks5 has no UDP ASSOCIATE)
	BypassLAN       bool
	RemoteDNS       string // resolver for hijacked DNS, "" = tcp://8.8.8.8 via tunnel
	ModernDNS       bool   // sing-box >= 1.11 route actions (kept for existing callers/tests)
	NewDNSFormat    *bool  // sing-box >= 1.12 typed DNS servers; nil defaults to ModernDNS (keeps legacy tests clean while defaulting modern configs to 1.12+)
	LogLevel        string
	Stack           string // "mixed" (default) | "gvisor" | "system"
	StrictRoute     bool   // tighter leak protection; may break LAN access
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

	// 1.12+ typed DNS servers are opt-in through NewDNSFormat; when the
	// caller does not know (old tests, hand-built options) it follows the
	// 1.11 action flag.
	newDNS := o.ModernDNS
	if o.NewDNSFormat != nil {
		newDNS = *o.NewDNSFormat
	}

	// A TCP-only upstream (the native wisteria core's armon/go-socks5 has
	// no UDP ASSOCIATE) turns every resolver into a TCP transport: a UDP
	// DNS server would time out through the SOCKS proxy, not just fall back.
	remoteDNS := firstNonEmpty(o.RemoteDNS, "tcp://8.8.8.8")
	if o.UpstreamTCPOnly {
		remoteDNS = ensureTCPDNS(remoteDNS)
	}

	// The server addresses must leave the tunnel at L3, in the kernel —
	// sing-box rejects bare addresses in ip_cidr, and a v6 address needs
	// /128 to mean "this host".
	serverCIDRs := normalizeIPCIDRs(o.ServerIPs)

	in := map[string]any{
		"type":           "tun",
		"tag":            "tun-in",
		"interface_name": tunInterfaceName,
		"address":        []string{"172.19.0.1/30"},
		"mtu":            9000,
		"auto_route":     true,
		"strict_route":   o.StrictRoute,
		"stack":          stack,
	}
	// Legacy sing-box needs inbound sniffing for the dns protocol rule;
	// modern ones sniff via the route rule action below.
	if !o.ModernDNS {
		in["sniff"] = true
	}
	if o.ModernDNS && len(serverCIDRs) > 0 {
		// Kernel-level exclusion: packets to the VPN server never enter
		// wintun at all, which removes the direct-connection loop the
		// route rule alone could not break (the helper's own sockets are
		// not covered by a process_path rule on every platform).
		in["route_exclude_address"] = serverCIDRs
	}

	var rules []any
	if o.ModernDNS {
		// Sniffing must be a rule of its own: without it the "protocol":
		// "dns" condition never matches and DNS silently dies over TUN.
		rules = append(rules, map[string]any{"action": "sniff"})
		rules = append(rules, map[string]any{
			"inbound": []string{"tun-in"}, "protocol": "dns", "action": "hijack-dns",
		})
		// Fallback for resolvers that speak plain DNS on port 53 before
		// sniffing has classified the connection (DoH/DoT bootstrap).
		rules = append(rules, map[string]any{
			"inbound": []string{"tun-in"}, "port": 53, "action": "hijack-dns",
		})
	} else {
		rules = append(rules, map[string]any{
			"inbound": []string{"tun-in"}, "protocol": "dns", "outbound": "dns-out",
		})
	}
	// Loopback never belongs in the proxy, even with BypassLAN off: local
	// services would otherwise be looped through the tunnel and back.
	rules = append(rules, map[string]any{
		"ip_cidr": []string{"127.0.0.0/8", "::1/128"}, "outbound": "direct",
	})
	if len(serverCIDRs) > 0 {
		rules = append(rules, map[string]any{"ip_cidr": serverCIDRs, "outbound": "direct"})
	}
	if o.CoreProcessPath != "" {
		// wisteria-client's bypass_ru direct connections originate on the
		// host: without this rule each one re-enters wisteria0 → sing-box →
		// SOCKS → wisteria-client forever.
		rules = append(rules, map[string]any{
			"process_path": []string{o.CoreProcessPath}, "outbound": "direct",
		})
	}
	if o.BypassLAN {
		rules = append(rules, map[string]any{"ip_is_private": true, "outbound": "direct"})
	}
	if o.UpstreamTCPOnly {
		// QUIC/HTTP3 would wait for a UDP ASSOCIATE the upstream cannot
		// answer, stalling Chrome for 10+ seconds before it falls back.
		// Rejecting UDP here makes the fallback immediate.
		if o.ModernDNS {
			rules = append(rules, map[string]any{"network": "udp", "action": "reject"})
		} else {
			rules = append(rules, map[string]any{"network": "udp", "outbound": "block"})
		}
	}

	proxy := map[string]any{"type": "socks", "tag": "proxy", "server": host, "server_port": port}
	if o.UpstreamTCPOnly {
		proxy["network"] = "tcp"
	}
	outbounds := []any{
		proxy,
		map[string]any{"type": "direct", "tag": "direct"},
	}
	if !o.ModernDNS {
		outbounds = append(outbounds, map[string]any{"type": "dns", "tag": "dns-out"})
	}
	if o.UpstreamTCPOnly && !o.ModernDNS {
		// The legacy generator has no reject action; the block outbound is
		// the 1.10 way to drop the same UDP.
		outbounds = append(outbounds, map[string]any{"type": "block", "tag": "block"})
	}

	route := map[string]any{
		"rules":                 rules,
		"final":                 "proxy",
		"auto_detect_interface": true,
	}

	dns := map[string]any{}
	if newDNS {
		remote, _ := buildSingBoxDNSServer("remote", remoteDNS, "proxy", true)
		dns["servers"] = []any{
			remote,
			// route.default_domain_resolver points here; without a local
			// server sing-box 1.14 refuses to start ("missing
			// route.default_domain_resolver") on domain-based proxies.
			map[string]any{"type": "local", "tag": "local"},
		}
		dns["final"] = "remote"
		route["default_domain_resolver"] = "local"
	} else {
		// 1.11 and older: the legacy address schema is still accepted.
		dns["servers"] = []any{map[string]any{
			"tag": "remote", "address": remoteDNS, "detour": "proxy",
		}}
	}

	cfg := map[string]any{
		"log":       map[string]any{"level": level, "timestamp": true},
		"inbounds":  []any{in},
		"outbounds": outbounds,
		"route":     route,
		"dns":       dns,
	}
	return json.MarshalIndent(cfg, "", "  ")
}

// buildSingBoxDNSServer renders one DNS server object for sing-box.
// When newFormat == false (< 1.12), emits legacy {"tag", "address", "detour"?}.
// When newFormat == true (>= 1.12), emits typed {"type", "tag", "server", "server_port"?, "path"?, "detour"?, "domain_resolver"?}.
// needsLocal is true when server is a domain name (not an IP literal), so
// the caller must make sure a "local" DNS server exists.
func buildSingBoxDNSServer(tag, rawAddr, detour string, newFormat bool) (srv map[string]any, needsLocal bool) {
	addr := strings.TrimSpace(rawAddr)
	if addr == "" {
		addr = "udp://8.8.8.8"
	}
	if !newFormat {
		srv = map[string]any{"tag": tag, "address": addr}
		if detour != "" {
			srv["detour"] = detour
		}
		return srv, false
	}
	if strings.EqualFold(addr, "local") {
		return map[string]any{"type": "local", "tag": tag}, false
	}
	u, err := parseDNSURL(addr)
	if err != nil || u.Host == "" {
		// A malformed resolver must not take the whole tunnel down: fall
		// back to the same default the empty value uses.
		srv = map[string]any{"type": "udp", "tag": tag, "server": "8.8.8.8"}
		if detour != "" {
			srv["detour"] = detour
		}
		return srv, false
	}
	typ := strings.ToLower(u.Scheme)
	switch typ {
	case "udp", "tcp", "tls", "https", "quic", "h3":
	default:
		typ = "udp"
	}
	host := u.Hostname()
	srv = map[string]any{"type": typ, "tag": tag, "server": host}
	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n >= 1 && n <= 65535 {
			srv["server_port"] = n
		}
	}
	// DoH paths carry meaning; "/" is the implicit default and sing-box
	// fills it in itself.
	if (typ == "https" || typ == "h3") && u.Path != "" && u.Path != "/" {
		srv["path"] = u.Path
	}
	if detour != "" {
		srv["detour"] = detour
	}
	if net.ParseIP(host) == nil && host != "" {
		srv["domain_resolver"] = "local"
		needsLocal = true
	}
	return srv, needsLocal
}

// parseDNSURL parses a resolver address, tolerating the forms users type:
// a bare IP/host, a bracketed or bare IPv6 literal, and a DoH URL with a
// path. A dummy udp:// scheme is prepended when none is present.
func parseDNSURL(addr string) (*url.URL, error) {
	withScheme := addr
	if !strings.Contains(withScheme, "://") {
		// A bare IPv6 literal has several colons and no scheme: without
		// brackets net/url reads the last group as a port.
		if strings.Count(withScheme, ":") >= 2 && !strings.HasPrefix(withScheme, "[") {
			withScheme = "[" + withScheme + "]"
		}
		withScheme = "udp://" + withScheme
	}
	return url.Parse(withScheme)
}

// ensureTCPDNS rewrites a resolver for a TCP-only SOCKS upstream: UDP and
// QUIC transports cannot use UDP ASSOCIATE (armon/go-socks5 does not
// implement it), while tcp/tls/https all run over TCP.
func ensureTCPDNS(rawAddr string) string {
	addr := strings.TrimSpace(rawAddr)
	if addr == "" {
		return "tcp://8.8.8.8"
	}
	lower := strings.ToLower(addr)
	switch {
	case strings.HasPrefix(lower, "udp://"):
		return "tcp://" + strings.TrimSpace(addr[len("udp://"):])
	case strings.HasPrefix(lower, "quic://"), strings.HasPrefix(lower, "h3://"):
		return "tcp://8.8.8.8"
	case strings.Contains(addr, "://"):
		return addr // tcp://, tls:// and https:// already run over TCP
	default:
		return "tcp://" + addr
	}
}

// normalizeIPCIDRs turns resolved server addresses into ip_cidr values:
// bare IPv4 gets /32, bare IPv6 gets /128, explicit CIDRs are validated and
// kept, junk is skipped. sing-box rejects a bare address in ip_cidr, and
// the old code emitted exactly that for every resolved server IP.
func normalizeIPCIDRs(ips []string) []string {
	out := make([]string, 0, len(ips))
	seen := make(map[string]bool, len(ips))
	for _, raw := range ips {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if strings.Contains(s, "/") {
			if _, _, err := net.ParseCIDR(s); err != nil {
				continue
			}
		} else {
			ip := net.ParseIP(s)
			if ip == nil {
				continue
			}
			if ip.To4() != nil {
				s += "/32"
			} else {
				s += "/128"
			}
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
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

// probeSingBoxCaps runs `sing-box version` and derives the config
// generation from the semver prefix. On any doubt it assumes modern
// upstream: current releases reject the legacy configs, so guessing old
// would guarantee a crash, while guessing new only misgenerates on an
// ancient binary.
func probeSingBoxCaps(bin string) SingBoxCaps {
	caps := SingBoxCaps{ModernActions: true, NewDNSFormat: true}
	out, err := runCoreOutput(bin, "version")
	if err != nil {
		return caps
	}
	major, minor, ok := parseSemverPrefix(string(out))
	if !ok {
		return caps
	}
	caps.Major, caps.Minor = major, minor
	caps.ModernActions = major > 1 || (major == 1 && minor >= 11)
	caps.NewDNSFormat = major > 1 || (major == 1 && minor >= 12)
	return caps
}

// singBoxModernDNS runs `sing-box version` and reports whether route rules
// with "action" are available (1.11+). Kept as a thin wrapper for callers
// that only need the one flag.
func singBoxModernDNS(bin string) bool { return probeSingBoxCaps(bin).ModernActions }

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
