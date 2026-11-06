package core

// Wisteria bundle links: the invite/link format minted by wisteria-peer and
// the panel (spec: internal/peers/invite.go in the main module):
//
//	wisteria://base64url({"v":1,"servers":[{"ep":"host:port","pub":"<b64>",
//	                   "transport":"tls?","tls_sni":"host?","tls_insecure":true?}],
//	                   "name":"label","key":"<b64>"})
//
// The per-server TLS keys (wave 4) are optional and additive to v1: absent
// or empty means the bare-TCP default, so links minted before the TLS
// transport existed decode unchanged.
//
// Two variants:
//   - invite (no "key"): public material only. The app generates a client
//     keypair and the admin has to authorize it before connect succeeds.
//   - link ("key" present): carries the client PRIVATE key, already
//     authorized on the server — the link is a credential and must be
//     handled like a secret (it lands in profiles.json, 0600).
//
// A bundle holds several endpoints: importing it expands to one profile per
// server via ParseLinkAll. ParseLink (single-profile API) accepts only a
// one-server bundle and refuses a multi one rather than silently dropping
// servers.

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	bundleLinkVersion = 1
	bundleMaxNameLen  = 64 // mirror of the minting side's bound
)

type bundleServer struct {
	EP  string `json:"ep"`
	Pub string `json:"pub"`
	// TLS transport (wave 4): optional per-server keys, all omitempty.
	// Absent or "tcp" = bare TCP; "tls" = the uTLS-wrapped dial. tls_sni
	// absent when empty or equal to the endpoint host; tls_insecure only
	// valid with transport = "tls". Old links without these keys decode to
	// the zero values and stay bare TCP — no version bump.
	Transport   string `json:"transport,omitempty"`
	TLSSNI      string `json:"tls_sni,omitempty"`
	TLSInsecure bool   `json:"tls_insecure,omitempty"`
}

type bundlePayload struct {
	V       int            `json:"v"`
	Servers []bundleServer `json:"servers"`
	Name    string         `json:"name"`
	Key     string         `json:"key"` // link variant only: client private key
}

// looksLikeBundle reports whether the line carries a wisteria scheme prefix
// (current or legacy spelling) with a bare base64 body — the bundle shape.
// Classic links always contain '@' (pinned pub) or a query/fragment, so they
// never match.
func looksLikeBundle(raw string) bool {
	s := strings.TrimSpace(raw)
	i := strings.Index(s, "://")
	if i < 0 {
		return false
	}
	scheme := s[:i]
	if !strings.EqualFold(scheme, string(TWisteria)) && !strings.EqualFold(scheme, "wysteria") {
		return false
	}
	body := s[i+3:]
	return body != "" && !strings.ContainsAny(body, "@?#")
}

// ParseWisteriaBundle decodes a bundle link into one profile per server.
// A non-bundle line yields a "not a bundle" error; a real bundle that fails
// validation gets a specific error — callers use looksLikeBundle to tell
// these apart.
func ParseWisteriaBundle(raw string) ([]Profile, error) {
	s := strings.TrimSpace(raw)
	i := strings.Index(s, "://")
	if i < 0 {
		return nil, errors.New("не ссылка (нет схемы)")
	}
	scheme, body := s[:i], strings.TrimSpace(s[i+3:])
	if !strings.EqualFold(scheme, string(TWisteria)) && !strings.EqualFold(scheme, "wysteria") {
		return nil, errors.New("не wisteria-ссылка")
	}
	payload := b64Flexible(body)
	if payload == "" {
		return nil, errors.New("тело ссылки не base64")
	}
	if !strings.HasPrefix(strings.TrimSpace(payload), "{") {
		return nil, errors.New("тело ссылки не JSON-набор")
	}
	var b bundlePayload
	// Unmarshal (not Decoder): trailing bytes after the object mean a
	// truncated or glued-together paste — reject, same as the minting side.
	if err := json.Unmarshal([]byte(payload), &b); err != nil {
		return nil, fmt.Errorf("JSON набора: %v", err)
	}
	if b.V == 0 {
		return nil, errors.New("в наборе нет версии (v)")
	}
	if b.V != bundleLinkVersion {
		return nil, fmt.Errorf("версия набора %d не поддерживается", b.V)
	}
	if len(b.Servers) == 0 {
		return nil, errors.New("в наборе нет ни одного сервера")
	}
	name := b.Name
	if r := []rune(name); len(r) > bundleMaxNameLen {
		name = string(r[:bundleMaxNameLen])
	}
	clientPriv := ""
	if k := strings.TrimSpace(b.Key); k != "" {
		raw32, err := decodeKey32(k)
		if err != nil {
			return nil, fmt.Errorf("клиентский ключ: %v", err)
		}
		clientPriv = base64.StdEncoding.EncodeToString(raw32)
	}
	out := make([]Profile, 0, len(b.Servers))
	for i, srv := range b.Servers {
		host, port, err := splitHostPort(strings.TrimSpace(srv.EP))
		if err != nil {
			return nil, fmt.Errorf("сервер %d (%s): %v", i+1, srv.EP, err)
		}
		if port == 0 {
			return nil, fmt.Errorf("сервер %d: нужен порт в %q", i+1, srv.EP)
		}
		pub := ""
		if pv := strings.TrimSpace(srv.Pub); pv != "" {
			raw32, err := decodeKey32(pv)
			if err != nil {
				return nil, fmt.Errorf("сервер %d: публичный ключ: %v", i+1, err)
			}
			pub = base64.StdEncoding.EncodeToString(raw32)
		}
		// TLS transport (wave 4): same rules the link parser applies to the
		// query dialect — a typo'd transport must not fall back to tcp, and
		// tls_sni/tls_insecure without transport = "tls" do not import.
		tr := strings.TrimSpace(srv.Transport)
		if err := ValidateTransport(tr); err != nil {
			return nil, fmt.Errorf("сервер %d (%s): %v", i+1, srv.EP, err)
		}
		sni := strings.TrimSpace(srv.TLSSNI)
		if err := ValidSNI(sni); err != nil {
			return nil, fmt.Errorf("сервер %d (%s): %v", i+1, srv.EP, err)
		}
		if (sni != "" || srv.TLSInsecure) && tr != "tls" {
			return nil, fmt.Errorf(`сервер %d (%s): tls_sni/tls_insecure требуют transport = "tls"`, i+1, srv.EP)
		}
		pname := name
		if pname == "" {
			pname = host
		}
		if len(b.Servers) > 1 {
			pname = fmt.Sprintf("%s · %d", pname, i+1)
		}
		p := Profile{
			Name:   pname,
			Type:   TWisteria,
			Host:   host,
			Port:   port,
			Fields: map[string]string{},
		}
		if tr == "tls" {
			p.Transport = "tls"
			p.TLSSNI = sni
			p.TLSInsecure = srv.TLSInsecure
		}
		if pub != "" {
			p.Fields["pub"] = pub
		}
		if clientPriv != "" {
			p.ClientPriv = clientPriv
		}
		// A one-server bundle keeps the raw link so store dedupe works;
		// a multi expansion drops Raw — store.Add dedupes by Raw and would
		// otherwise collapse servers 2..N onto the first match.
		if len(b.Servers) == 1 {
			p.Raw = raw
		}
		out = append(out, p)
	}
	return out, nil
}

// ParseLinkAll parses one pasted line into 1..N profiles: a wisteria bundle
// expands to one profile per server, any other share link yields exactly
// one. Import paths (links modal, subscriptions) must use this, not
// ParseLink, so multi-server bundles are not truncated.
func ParseLinkAll(raw string) ([]Profile, error) {
	if looksLikeBundle(raw) {
		return ParseWisteriaBundle(raw)
	}
	p, err := ParseLink(raw)
	if err != nil {
		return nil, err
	}
	return []Profile{p}, nil
}

// decodeKey32 decodes a base64 (any common variant) 32-byte key.
func decodeKey32(s string) ([]byte, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "=")
	if s == "" {
		return nil, errors.New("пустой ключ")
	}
	for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			if len(b) != 32 {
				return nil, fmt.Errorf("ожидалось 32 байта, получено %d", len(b))
			}
			return b, nil
		}
	}
	return nil, errors.New("не base64")
}
