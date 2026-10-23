package core

// User-defined routing lists (direct / block). The settings UI takes one
// entry per line; commas and semicolons work too. A domain entry matches the
// domain and all its subdomains (renderer-side: sing-box domain_suffix, xray
// "domain:" prefix). IP entries may be plain addresses or CIDRs.

import (
	"net"
	"strings"
)

// parseRouteList splits a raw settings string into domain entries and
// IP/CIDR entries. Junk (empty lines, comments, invalid tokens) is skipped.
func parseRouteList(s string) (domains, cidrs []string) {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		// Comments are line-scoped, so check before tokenising.
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		for _, tok := range strings.FieldsFunc(line, func(r rune) bool {
			return r == ',' || r == ';' || r == ' ' || r == '\t'
		}) {
			e := strings.TrimSuffix(strings.TrimSpace(tok), ".") // tolerate "example.com."
			if e == "" {
				continue
			}
			if strings.Contains(e, "/") {
				if _, _, err := net.ParseCIDR(e); err == nil {
					cidrs = append(cidrs, e)
				}
				continue
			}
			if net.ParseIP(e) != nil {
				cidrs = append(cidrs, e)
				continue
			}
			if validRouteDomain(e) {
				domains = append(domains, strings.ToLower(e))
			}
		}
	}
	return domains, cidrs
}

// validRouteDomain: loose sanity check — letters/digits/hyphens/underscores
// separated by dots. Real validation is up to the core's config loader.
func validRouteDomain(d string) bool {
	if d == "" || len(d) > 253 || strings.ContainsAny(d, "/: ") {
		return false
	}
	for _, part := range strings.Split(d, ".") {
		if part == "" {
			return false
		}
		for _, r := range part {
			ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_'
			if !ok {
				return false
			}
		}
	}
	return true
}
