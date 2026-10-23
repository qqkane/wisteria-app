package core

// Profile: one server entry of any supported protocol. For wisteria the
// profile carries enough information to render a full client TOML; for
// sing-box protocols it carries the fields of the share link, rendered
// into a sing-box JSON config at connect time.

import (
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
}

// Endpoint returns host:port.
func (p Profile) Endpoint() string { return joinHostPort(p.Host, p.Port) }

func joinHostPort(host string, port int) string {
	if port == 0 {
		return host
	}
	if isIPv6(host) {
		return "[" + host + "]:" + itoa(port)
	}
	return host + ":" + itoa(port)
}
