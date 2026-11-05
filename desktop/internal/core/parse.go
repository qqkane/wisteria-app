package core

// Share-link parsers. The share formats follow the de-facto standards used
// by v2rayN / NekoBox / Shadowrocket:
//
//	ss://        SIP002 (base64 method:password@host:port) + legacy full-base64
//	vmess://     base64 JSON (v2rayN format)
//	vless://     URI with query params (security/reality/transport)
//	trojan://    URI, password in userinfo
//	hysteria2:// URI (hy2:// alias), password in userinfo
//	tuic://      URI, uuid:password in userinfo
//	wisteria://  this app's format: base64(server pubkey)@host:port?obf params#name
//
// Parsing is done manually (not url.Parse on the whole link) because base64
// userinfo contains '+', '/' and '=' which are not legal URL characters and
// make url.Parse fail on real-world links.

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ParseLink parses one share link into a Profile (no id; store.Add assigns).
func ParseLink(raw string) (Profile, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, "\n", "")
	switch {
	case strings.HasPrefix(raw, "hy2://"):
		raw = "hysteria2://" + strings.TrimPrefix(raw, "hy2://")
	}

	scheme, rest, ok := strings.Cut(raw, "://")
	if !ok {
		return Profile{}, fmt.Errorf("не ссылка с схемой (scheme://...)")
	}
	// Links minted before the 2026-10-23 rename (Wysteria -> Wisteria) are still
	// in chat logs and in links.toml on the servers. The rename never touched
	// the link body, so we keep accepting the old prefix while always writing
	// new links with wisteria://.
	if strings.EqualFold(scheme, "wysteria") {
		scheme = string(TWisteria)
	}
	// Bundle links (wisteria://base64(JSON), the panel/wisteria-peer format)
	// must be caught before the generic URI machinery turns the base64 body
	// into a bogus host. A one-server bundle parses right here; a multi one
	// is refused — import paths use ParseLinkAll, which expands it.
	if ProfileType(scheme) == TWisteria && looksLikeBundle(raw) {
		profs, err := ParseWisteriaBundle(raw)
		if err != nil {
			return Profile{}, err
		}
		if len(profs) == 1 {
			return profs[0], nil
		}
		return Profile{}, fmt.Errorf("набор содержит %d серверов — при импорте в «Ссылки» или подписку он развернётся в %d профилей", len(profs), len(profs))
	}
	// Fragment (display name) comes after '#' anywhere in the rest.
	name := ""
	if i := strings.LastIndex(rest, "#"); i >= 0 {
		name = unescape(rest[i+1:])
		rest = rest[:i]
	}
	// Query comes after '?'.
	query := map[string]string{}
	if i := strings.Index(rest, "?"); i >= 0 {
		var err error
		query, err = parseQuery(rest[i+1:])
		if err != nil {
			return Profile{}, fmt.Errorf("параметры: %v", err)
		}
		rest = rest[:i]
	}
	// Userinfo comes before the LAST '@' (base64 may itself contain '@'-free
	// but '='-padded data; hosts may not contain '@', so last is safest).
	userinfo := ""
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		userinfo = rest[:i]
		rest = rest[i+1:]
	}
	host, port, err := splitHostPort(rest)
	if err != nil {
		return Profile{}, err
	}

	base := Profile{
		Name:   name, // may be empty; defaulted after parsing
		Host:   host,
		Port:   port,
		Raw:    raw,
		Fields: map[string]string{},
	}

	res, err := parseDispatch(base, ProfileType(scheme), userinfo, rest, query)
	if err != nil {
		return Profile{}, err
	}
	if res.Name == "" {
		res.Name = res.Host + ":" + strconv.Itoa(res.Port)
	}
	return res, nil
}

func parseDispatch(p Profile, scheme ProfileType, userinfo, rest string, query map[string]string) (Profile, error) {
	switch scheme {
	case TSS:
		return parseSS(p, userinfo, rest, query)
	case TVMess:
		return parseVMess(p, userinfo, rest)
	case TVLESS:
		return parseVLESS(p, userinfo, query)
	case TTrojan:
		return parseTrojan(p, userinfo, query)
	case THysteria2:
		return parseHysteria2(p, userinfo, query)
	case TTuic:
		return parseTuic(p, userinfo, query)
	case TWisteria:
		return parseWisteria(p, userinfo, query)
	}
	return Profile{}, fmt.Errorf("неподдерживаемый протокол %q (поддерживаются ss, vmess, vless, trojan, hysteria2, tuic, wisteria)", scheme)
}

// ---- ss ----

func parseSS(p Profile, userinfo, rest string, query map[string]string) (Profile, error) {
	if query["plugin"] != "" {
		return Profile{}, fmt.Errorf("ss: плагины (obfs/v2ray-plugin) пока не поддерживаются")
	}
	// SIP002: base64(method:password)@host:port. Legacy: the whole body is
	// base64(method:password@host:port) and there is no userinfo.
	payload := userinfo
	if payload == "" {
		payload = rest
	}
	dec := b64Flexible(payload)
	if dec == "" {
		return Profile{}, fmt.Errorf("ss: не удалось декодировать base64-часть")
	}
	if userinfo == "" {
		// Legacy body: method:password@host:port
		cred, hostport, found := strings.Cut(dec, "@")
		if !found {
			return Profile{}, fmt.Errorf("ss: в legacy-формате ожидался @ внутри base64")
		}
		method, password, found := strings.Cut(cred, ":")
		if !found {
			return Profile{}, fmt.Errorf("ss: ожидался формат method:password")
		}
		h, port, err := splitHostPort(hostport)
		if err != nil {
			return Profile{}, fmt.Errorf("ss: %v", err)
		}
		p.Host, p.Port = h, port
		p.Type = TSS
		p.Fields["method"] = method
		p.Fields["password"] = password
		return p, nil
	}
	method, password, found := strings.Cut(dec, ":")
	if !found {
		return Profile{}, fmt.Errorf("ss: ожидался формат method:password")
	}
	p.Type = TSS
	p.Fields["method"] = method
	p.Fields["password"] = password
	return p, nil
}

// ---- vmess (base64 JSON, v2rayN) ----

type vmessJSON struct {
	Ps   string      `json:"ps"`
	Add  string      `json:"add"`
	Port json.Number `json:"port"`
	ID   string      `json:"id"`
	Aid  json.Number `json:"aid"`
	Scy  string      `json:"scy"`
	Net  string      `json:"net"`
	Type string      `json:"type"`
	Host string      `json:"host"`
	Path string      `json:"path"`
	TLS  string      `json:"tls"`
	SNI  string      `json:"sni"`
}

func parseVMess(p Profile, userinfo, rest string) (Profile, error) {
	payload := userinfo
	if payload == "" {
		payload = rest
	}
	dec := b64Flexible(payload)
	if dec == "" {
		return Profile{}, fmt.Errorf("vmess: не удалось декодировать base64-тело")
	}
	var v vmessJSON
	if err := json.Unmarshal([]byte(dec), &v); err != nil {
		return Profile{}, fmt.Errorf("vmess: JSON: %v", err)
	}
	port, err := v.Port.Int64()
	if err != nil || port <= 0 {
		return Profile{}, fmt.Errorf("vmess: некорректный порт %q", v.Port.String())
	}
	p.Type = TVMess
	p.Host = firstNonEmpty(v.Add, p.Host)
	p.Port = int(port)
	if p.Name == "" && v.Ps != "" {
		p.Name = v.Ps
	}
	p.Fields["uuid"] = v.ID
	p.Fields["security"] = firstNonEmpty(v.Scy, "auto")
	p.Fields["alter_id"] = firstNonEmpty(v.Aid.String(), "0")
	p.Fields["transport"] = firstNonEmpty(v.Net, "tcp")
	if v.Host != "" {
		p.Fields["ws_host"] = v.Host
	}
	if v.Path != "" {
		p.Fields["path"] = v.Path
	}
	if v.TLS == "tls" || strings.EqualFold(v.TLS, "true") {
		p.Fields["tls"] = "1"
		p.Fields["sni"] = firstNonEmpty(v.SNI, v.Host)
	}
	return p, nil
}

// ---- vless ----

func parseVLESS(p Profile, userinfo string, q map[string]string) (Profile, error) {
	if userinfo == "" {
		return Profile{}, fmt.Errorf("vless: пустой uuid")
	}
	p.Type = TVLESS
	p.Fields["uuid"] = userinfo
	p.Fields["encryption"] = firstNonEmpty(q["encryption"], "none")
	p.Fields["security"] = q["security"] // none | tls | reality
	p.Fields["sni"] = q["sni"]
	p.Fields["fingerprint"] = q["fp"]
	p.Fields["public_key"] = q["pbk"]
	p.Fields["short_id"] = q["sid"]
	p.Fields["spider_x"] = q["spx"]
	p.Fields["flow"] = q["flow"]
	p.Fields["transport"] = firstNonEmpty(q["type"], "tcp")
	p.Fields["path"] = q["path"]
	p.Fields["service_name"] = q["serviceName"]
	p.Fields["ws_host"] = q["host"]
	p.Fields["alpn"] = q["alpn"]
	return p, nil
}

// ---- trojan ----

func parseTrojan(p Profile, userinfo string, q map[string]string) (Profile, error) {
	if userinfo == "" {
		return Profile{}, fmt.Errorf("trojan: пустой пароль")
	}
	p.Type = TTrojan
	p.Fields["password"] = unescape(userinfo)
	p.Fields["sni"] = q["sni"]
	p.Fields["transport"] = firstNonEmpty(q["type"], "tcp")
	p.Fields["path"] = q["path"]
	p.Fields["ws_host"] = q["host"]
	p.Fields["alpn"] = q["alpn"]
	p.Fields["security"] = "tls" // trojan is always TLS
	return p, nil
}

// ---- hysteria2 ----

func parseHysteria2(p Profile, userinfo string, q map[string]string) (Profile, error) {
	if userinfo == "" {
		return Profile{}, fmt.Errorf("hysteria2: пустой пароль аутентификации")
	}
	p.Type = THysteria2
	p.Fields["password"] = unescape(userinfo)
	p.Fields["sni"] = q["sni"]
	p.Fields["insecure"] = normalizeBool(q["insecure"])
	p.Fields["obfs"] = q["obfs"]
	p.Fields["obfs_password"] = q["obfs-password"]
	if p.Fields["obfs_password"] == "" {
		p.Fields["obfs_password"] = q["obfs_password"]
	}
	p.Fields["alpn"] = q["alpn"]
	p.Fields["pin_sha256"] = q["pinSHA256"]
	return p, nil
}

// ---- tuic ----

func parseTuic(p Profile, userinfo string, q map[string]string) (Profile, error) {
	uuid, pass, _ := strings.Cut(unescape(userinfo), ":")
	if uuid == "" {
		return Profile{}, fmt.Errorf("tuic: пустой uuid")
	}
	p.Type = TTuic
	p.Fields["uuid"] = uuid
	p.Fields["password"] = pass
	p.Fields["congestion_control"] = firstNonEmpty(q["congestion_control"], "bbr")
	p.Fields["udp_relay_mode"] = firstNonEmpty(q["udp_relay_mode"], "native")
	p.Fields["sni"] = q["sni"]
	p.Fields["alpn"] = q["alpn"]
	p.Fields["congestion_control"] = firstNonEmpty(q["congestion_control"], p.Fields["congestion_control"])
	return p, nil
}

// ---- wisteria ----
//
//	wisteria://BASE64URL(server_pub)@host:port?obf_mu=6.7&obf_sigma=0.6&pad=64-1448&jitter_us=1&coalesce_ms=5&mimic=1&filler=80-400&filler_ratio=5&rekey=16777216#Имя
//
// The server public key is optional: without it the first handshake runs XX
// and the core pins the key itself (serverkey cache). All obfuscation
// parameters are optional; omitted ones keep the client defaults, which
// must match the server's config anyway.

func parseWisteria(p Profile, userinfo string, q map[string]string) (Profile, error) {
	if p.Port == 0 {
		// A scheme prefix followed by non-URI junk used to import as a
		// profile with a base64 "host" and port 0; that is never valid.
		return Profile{}, errors.New("wisteria: нужен порт сервера (host:port)")
	}
	p.Type = TWisteria
	pub := ""
	if userinfo != "" {
		raw := unescape(userinfo)
		if b, err := base64.StdEncoding.DecodeString(raw); err == nil && len(b) == 32 {
			pub = base64.StdEncoding.EncodeToString(b)
		} else if b, err := base64.RawURLEncoding.DecodeString(raw); err == nil && len(b) == 32 {
			pub = base64.StdEncoding.EncodeToString(b)
		} else if b, err := base64.URLEncoding.DecodeString(raw); err == nil && len(b) == 32 {
			pub = base64.StdEncoding.EncodeToString(b)
		} else if b, err := base64.RawStdEncoding.DecodeString(raw); err == nil && len(b) == 32 {
			pub = base64.StdEncoding.EncodeToString(b)
		} else {
			return Profile{}, fmt.Errorf("wisteria: публичный ключ сервера должен быть base64 из 32 байт")
		}
	}
	if pub != "" {
		p.Fields["pub"] = pub
	}
	// Numeric obfuscation fields land in the client TOML as raw tokens after
	// "=" (wisteria_conf.go). A percent-encoded newline (%0A) survives
	// unescape, so an unvalidated value would inject TOML tables — every
	// field is checked here at import, and the renderer re-checks (the manual
	// form bypasses this parser).
	pair := func(key, lo, hi string) error {
		v := q[key]
		if v == "" {
			return nil
		}
		a, b, ok := strings.Cut(v, "-")
		if !ok {
			return fmt.Errorf("wisteria: %s ожидался как min-max", key)
		}
		for _, x := range []string{a, b} {
			if !validObfUint(x) {
				return fmt.Errorf("wisteria: %s = %q — нужны неотрицательные целые min-max", key, v)
			}
		}
		p.Fields[lo], p.Fields[hi] = a, b
		return nil
	}
	if err := pair("pad", "min_pad", "max_pad"); err != nil {
		return Profile{}, err
	}
	if err := pair("filler", "filler_min", "filler_max"); err != nil {
		return Profile{}, err
	}
	for _, k := range obfUintFields {
		if v := q[k]; v != "" {
			if !validObfUint(v) {
				return Profile{}, fmt.Errorf("wisteria: %s = %q — нужно неотрицательное целое", k, v)
			}
			p.Fields[k] = v
		}
	}
	for _, k := range obfFloatFields {
		if v := q[k]; v != "" {
			if !validObfFloat(v) {
				return Profile{}, fmt.Errorf("wisteria: %s = %q — нужно число (TOML float)", k, v)
			}
			p.Fields[k] = v
		}
	}
	return p, nil
}

// ParseWisteriaTOML imports a full client config (the configs/client.toml
// format). Everything the tunnel needs is carried over: endpoint, keys,
// obfuscation. The client private key from the file is kept — regenerating
// it would orphan the profile on the server's authorized_peers list.
func ParseWisteriaTOML(text string) (Profile, error) {
	var doc struct {
		Server struct {
			Endpoint string `toml:"endpoint"`
		} `toml:"server"`
		Servers []struct {
			Endpoint  string `toml:"endpoint"`
			PublicKey string `toml:"public_key"`
		} `toml:"servers"`
		Crypto struct {
			StaticPrivate      string `toml:"static_private"`
			ServerStaticPublic string `toml:"server_static_public"`
		} `toml:"crypto"`
		Obfuscation struct {
			MuLog       float64 `toml:"mu_log"`
			Sigma       float64 `toml:"sigma"`
			MinPad      int     `toml:"min_pad"`
			MaxPad      int     `toml:"max_pad"`
			JitterMicro int     `toml:"jitter_micro_ms"`
			JitterMean  int     `toml:"jitter_mean_ms"`
			Coalesce    int     `toml:"coalesce_ms"`
			MimicTLS    int     `toml:"mimic_tls"`
			FillerMin   int     `toml:"filler_min"`
			FillerMax   int     `toml:"filler_max"`
			FillerRatio int     `toml:"filler_ratio"`
			DirBalance  float64 `toml:"dir_balance"`
			RekeyFrames uint64  `toml:"rekey_frames"`
		} `toml:"obfuscation"`
	}
	if err := tomlUnmarshal([]byte(text), &doc); err != nil {
		return Profile{}, fmt.Errorf("toml: %v", err)
	}
	ep := ""
	pub := ""
	if len(doc.Servers) > 0 && doc.Servers[0].Endpoint != "" {
		ep = doc.Servers[0].Endpoint
		pub = doc.Servers[0].PublicKey
	} else {
		ep = doc.Server.Endpoint
	}
	if ep == "" {
		return Profile{}, fmt.Errorf("toml: не найден endpoint (ни [server], ни [[servers]])")
	}
	if doc.Crypto.StaticPrivate == "" {
		return Profile{}, fmt.Errorf("toml: не найден crypto.static_private")
	}
	host, port, err := splitHostPort(ep)
	if err != nil {
		return Profile{}, err
	}
	p := Profile{
		Name:       host + ":" + strconv.Itoa(port),
		Type:       TWisteria,
		Host:       host,
		Port:       port,
		ClientPriv: doc.Crypto.StaticPrivate,
		Fields:     map[string]string{},
	}
	if pub == "" {
		pub = doc.Crypto.ServerStaticPublic
	}
	if pub != "" {
		p.Fields["pub"] = pub
	}
	o := doc.Obfuscation
	if o.MuLog != 0 {
		p.Fields["obf_mu"] = trimFloat(o.MuLog)
	}
	if o.Sigma != 0 {
		p.Fields["obf_sigma"] = trimFloat(o.Sigma)
	}
	if o.MinPad != 0 || o.MaxPad != 0 {
		p.Fields["min_pad"] = strconv.Itoa(o.MinPad)
		p.Fields["max_pad"] = strconv.Itoa(o.MaxPad)
	}
	if o.JitterMicro != 0 {
		p.Fields["jitter_us"] = strconv.Itoa(o.JitterMicro)
	}
	if o.JitterMean != 0 {
		p.Fields["jitter_ms"] = strconv.Itoa(o.JitterMean)
	}
	if o.Coalesce != 0 {
		p.Fields["coalesce_ms"] = strconv.Itoa(o.Coalesce)
	}
	if o.MimicTLS != 0 {
		p.Fields["mimic"] = strconv.Itoa(o.MimicTLS)
	}
	if o.FillerMin != 0 || o.FillerMax != 0 {
		p.Fields["filler_min"] = strconv.Itoa(o.FillerMin)
		p.Fields["filler_max"] = strconv.Itoa(o.FillerMax)
	}
	if o.FillerRatio != 0 {
		p.Fields["filler_ratio"] = strconv.Itoa(o.FillerRatio)
	}
	if o.DirBalance != 0 {
		p.Fields["dir_balance"] = trimFloat(o.DirBalance)
	}
	if o.RekeyFrames != 0 {
		p.Fields["rekey"] = strconv.FormatUint(o.RekeyFrames, 10)
	}
	return p, nil
}

// ---- helpers ----

// b64Flexible decodes base64 in any of the four common variants exporters
// produce (std/url, padded or not). Padding is stripped so the raw decoders
// handle both cases.
func b64Flexible(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "=")
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return string(b)
	}
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return string(b)
	}
	return ""
}

// splitHostPort parses host:port tolerating base64 junk before it (already
// cut by the caller), brackets for IPv6 and a missing port.
func splitHostPort(s string) (string, int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0, fmt.Errorf("пустой host:port")
	}
	if host, portStr, err := net.SplitHostPort(s); err == nil {
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return "", 0, fmt.Errorf("некорректный порт %q", portStr)
		}
		return host, port, nil
	}
	// No port.
	if isIPv6(s) || net.ParseIP(s) != nil || !strings.Contains(s, ":") {
		return s, 0, nil
	}
	return "", 0, fmt.Errorf("не удалось разобрать host:port из %q", s)
}

func isIPv6(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && strings.Contains(host, ":")
}

func parseQuery(qs string) (map[string]string, error) {
	out := map[string]string{}
	for _, kv := range strings.Split(qs, "&") {
		if kv == "" {
			continue
		}
		k, v, _ := strings.Cut(kv, "=")
		k = unescape(k)
		if k == "" {
			continue
		}
		out[k] = unescape(v)
	}
	return out, nil
}

// unescape percent-decoding where '+' is literal (query/fragment contexts).
func unescape(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	if u, err := urlPathUnescape(s); err == nil {
		return u
	}
	return s
}

func normalizeBool(v string) string {
	switch strings.ToLower(v) {
	case "1", "true", "yes":
		return "1"
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func itoa(n int) string { return strconv.Itoa(n) }
