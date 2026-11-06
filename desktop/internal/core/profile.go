package core

// Profile: one server entry of any supported protocol. For wisteria the
// profile carries enough information to render a full client TOML; for
// sing-box protocols it carries the fields of the share link, rendered
// into a sing-box JSON config at connect time.

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// ProfileType is the protocol discriminator. "wisteria" runs the native
// core (bin/wisteria-client), everything else runs through sing-box.
type ProfileType string

const (
	TWisteria  ProfileType = "wisteria"
	TSS        ProfileType = "ss"
	TVMess     ProfileType = "vmess"
	TVLESS     ProfileType = "vless"
	TTrojan    ProfileType = "trojan"
	THysteria2 ProfileType = "hysteria2"
	TTuic      ProfileType = "tuic"
)

// Supported reports whether the type is known to the app.
func Supported(t ProfileType) bool {
	switch t {
	case TWisteria, TSS, TVMess, TVLESS, TTrojan, THysteria2, TTuic:
		return true
	}
	return false
}

// SingBox reports whether the type is rendered into a sing-box config
// (everything except the native wisteria core).
func SingBox(t ProfileType) bool { return t != TWisteria && Supported(t) }

// Profile is one connection entry. Fields holds the protocol-specific
// parameters keyed by short names (see parse.go): password, uuid, method,
// sni, security, transport, path, pub, obf_mu, ... The frontend edits the
// same map, so no field is hidden from the UI.
type Profile struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       ProfileType       `json:"type"`
	Host       string            `json:"host"`
	Port       int               `json:"port"`
	Raw        string            `json:"raw,omitempty"`
	Fields     map[string]string `json:"fields,omitempty"`
	ClientPriv string            `json:"client_priv,omitempty"` // wisteria X25519 private key (base64)
	CreatedAt  time.Time         `json:"created_at"`
	LatencyMs  int64             `json:"latency_ms,omitempty"` // last TCP ping; -1 = failed
	SourceID   string            `json:"sub_id,omitempty"`     // subscription that imported this profile
	// TLS transport (wisteria wave 4): the endpoint may sit behind a real
	// TLS listener (server crypto/tls, client uTLS). Zero values mean the
	// bare-TCP default, so profiles saved before TLS have no keys and stay
	// tcp — backward compatible by construction. Only wisteria profiles use
	// them; the sing-box family keeps its transport in Fields.
	Transport   string `json:"transport,omitempty"`    // "" | "tls" ("tcp" is normalized away)
	TLSSNI      string `json:"tls_sni,omitempty"`      // optional ServerName override; "" = endpoint host
	TLSInsecure bool   `json:"tls_insecure,omitempty"` // skip certificate verification (lab only)
}

// Endpoint returns host:port.
func (p Profile) Endpoint() string { return joinHostPort(p.Host, p.Port) }

// ValidateTransport reports whether s is a transport value this build
// speaks: "" (the bare-TCP default), "tcp" or "tls" (wave 4). Mirrors the
// core's config loader — a typo'd transport must not silently fall back to
// tcp and change what lands on the wire.
func ValidateTransport(s string) error {
	switch s {
	case "", "tcp", "tls":
		return nil
	default:
		return fmt.Errorf(`transport должен быть "tcp" или "tls", получено %q`, s)
	}
}

// ValidSNI reports whether s is empty or a sane TLS ServerName: RFC 1123
// host labels (letters/digits/hyphens), each 1..63 chars, no underscores,
// no empty labels, at most 253 chars total. An IP literal is rejected on
// purpose: RFC 6066 forbids literal addresses in the ServerName extension,
// and name verification against a DNS-san certificate would fail regardless
// — an endpoint that can only work by IP belongs to tls_insecure territory.
// Same rules as the protocol's config loader.
func ValidSNI(s string) error {
	if s == "" {
		return nil
	}
	if len(s) > 253 {
		return fmt.Errorf("tls_sni %q длиннее 253 символов", s)
	}
	if net.ParseIP(s) != nil {
		return fmt.Errorf("tls_sni %q — IP-адрес: нужен домен (или уберите tls_sni — по умолчанию берётся хост endpoint)", s)
	}
	for i, label := range strings.Split(s, ".") {
		if label == "" {
			return fmt.Errorf("tls_sni %q: пустая метка (позиция %d)", s, i+1)
		}
		if len(label) > 63 {
			return fmt.Errorf("tls_sni %q: метка %q длиннее 63 символов", s, label)
		}
		for _, r := range label {
			if !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' || r == '-') {
				return fmt.Errorf("tls_sni %q: символ %q недопустим в имени хоста", s, string(r))
			}
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("tls_sni %q: метка %q не должна начинаться или заканчиваться дефисом", s, label)
		}
	}
	return nil
}

// normalizeTLS canonicalizes and validates the TLS fields of a wisteria
// profile: "tcp" collapses onto "" (one canonical default), the SNI is
// trimmed, and tls_sni/tls_insecure without transport = "tls" are rejected —
// the same combo rule the link parsers and the core's config loader apply.
// A hand-edited store or the manual form reaches this without any parser,
// so every save path funnels through it (store.Add and the TOML renderer).
func (p *Profile) normalizeTLS() error {
	if p.Type != TWisteria {
		return nil
	}
	p.Transport = strings.TrimSpace(p.Transport)
	switch p.Transport {
	case "", "tcp":
		p.Transport = ""
	case "tls":
	default:
		return ValidateTransport(p.Transport)
	}
	p.TLSSNI = strings.TrimSpace(p.TLSSNI)
	if err := ValidSNI(p.TLSSNI); err != nil {
		return err
	}
	if p.Transport != "tls" && (p.TLSSNI != "" || p.TLSInsecure) {
		return fmt.Errorf(`tls_sni/tls_insecure требуют transport = "tls"`)
	}
	return nil
}

func joinHostPort(host string, port int) string {
	if port == 0 {
		return host
	}
	if isIPv6(host) {
		return "[" + host + "]:" + itoa(port)
	}
	return host + ":" + itoa(port)
}
