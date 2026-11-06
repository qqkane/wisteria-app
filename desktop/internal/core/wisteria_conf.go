package core

// Renders a full wisteria client TOML from a profile. Field names match
// configs/client.toml.example; the core validates on load, so a wrong value
// surfaces in the core log rather than silently degrading the tunnel.

import (
	"fmt"
	"strconv"
	"strings"
)

// obfUintFields / obfFloatFields are the profile fields rendered into the
// client TOML as RAW tokens after "=" (mu_log = %s). Anything that is not a
// clean TOML number would inject or kill config tables (a percent-encoded
// newline survives link unescaping, and Fields are also editable in the
// manual form and by a hand-edited store) — both parsers and the renderer
// below enforce this.
var (
	obfUintFields  = []string{"min_pad", "max_pad", "jitter_us", "jitter_ms", "coalesce_ms", "mimic", "filler_min", "filler_max", "filler_ratio", "rekey", "pow"}
	obfFloatFields = []string{"obf_mu", "obf_sigma", "dir_balance"}
)

// validObfUint: non-negative int64, digits only (the TOML integer grammar).
func validObfUint(v string) bool {
	_, err := strconv.ParseUint(v, 10, 63)
	return err == nil
}

// validObfFloat: strict TOML decimal float — digits [. digits] [e[+-]digits],
// no sign, no NaN/Inf/hex/underscores (strconv.ParseFloat is looser than the
// TOML grammar; this scanner is exactly it).
func validObfFloat(s string) bool {
	i := 0
	digits := func() bool {
		n := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
			n++
		}
		return n > 0
	}
	if !digits() {
		return false
	}
	if i < len(s) && s[i] == '.' {
		i++
		if !digits() {
			return false
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		if !digits() {
			return false
		}
	}
	return i == len(s)
}

// checkObfFields verifies every numeric obfuscation field of the profile;
// the error names the field and the bad value.
func checkObfFields(f map[string]string) error {
	for _, k := range obfUintFields {
		if v := f[k]; v != "" && !validObfUint(v) {
			return fmt.Errorf("wisteria: %s = %q — нужно неотрицательное целое", k, v)
		}
	}
	for _, k := range obfFloatFields {
		if v := f[k]; v != "" && !validObfFloat(v) {
			return fmt.Errorf("wisteria: %s = %q — нужно число (TOML float)", k, v)
		}
	}
	return nil
}

// WisteriaTOML renders the client config. socksListen is the local SOCKS5
// address; cachePath is the per-profile server-key cache (XX→IK pinning).
func WisteriaTOML(p Profile, socksListen, cachePath string) (string, error) {
	if p.Type != TWisteria {
		return "", fmt.Errorf("профиль %q не wisteria", p.ID)
	}
	if p.ClientPriv == "" {
		return "", fmt.Errorf("у профиля нет client key (static_private)")
	}
	// Render-time validation, not just parse-time: the manual form and a
	// hand-edited store put fields in without the share-link parser.
	if err := checkObfFields(p.Fields); err != nil {
		return "", err
	}
	if err := p.normalizeTLS(); err != nil {
		return "", err
	}
	tls := p.Transport == "tls"
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	w("[client]")
	w("socks5_listen = %q", socksListen)
	w("")
	// The loader requires the legacy [server] endpoint even when the pool is
	// given via [[servers]] (it validates before wrapping single entries).
	w("[server]")
	w("endpoint  = %q", p.Endpoint())
	if tls {
		w("transport = \"tls\"")
	} else {
		w("transport = \"tcp\"")
	}
	w("")
	pub := p.Fields["pub"]
	// The [[servers]] pool row carries the pinned key when the profile has
	// one. A TLS profile without a pub still needs the row: the legacy
	// [server] block has no tls_sni/tls_insecure fields, so the pool row is
	// the only place the TLS settings can ride (an empty public_key keeps
	// the XX-pin-on-first-connect behaviour).
	if pub != "" || tls {
		w("[[servers]]")
		w("endpoint   = %q", p.Endpoint())
		w("weight     = 1")
		if pub != "" {
			w("public_key = %q", pub)
		}
		// TLS rows (wave 4) are emitted only when non-default, so a tcp
		// profile renders byte-identical to the pre-wave-4 output. tls_sni
		// is omitted when it equals the endpoint host: the loader defaults
		// it to exactly that value.
		if tls {
			w("transport  = \"tls\"")
			if sni := p.TLSSNI; sni != "" && sni != p.Host {
				w("tls_sni    = %q", sni)
			}
			if p.TLSInsecure {
				w("tls_insecure = true")
			}
		}
		w("")
	}
	if pub != "" {
		// A pinned key goes into the [[servers]] row (per-endpoint pin, IK
		// from the first connect) and into crypto.server_static_public.
		w("[crypto]")
		w("pattern              = \"auto\"")
		w("static_private       = %q", p.ClientPriv)
		w("server_static_public = %q", pub)
	} else {
		w("[crypto]")
		w("pattern              = \"auto\"")
		w("static_private       = %q", p.ClientPriv)
		w("server_static_public = \"\"")
	}
	w("")
	w("[relay]")
	w("mode = \"direct\"")
	w("selection = \"latency\"")
	w("backoff_initial_ms = 500")
	w("backoff_max_ms = 30000")
	w("probe_interval_sec = 30")
	w("probe_timeout_sec = 3")
	w("")

	// Obfuscation: emit only what the profile carries — the rest keeps the
	// core defaults, which match a server deployed with defaults.
	f := p.Fields
	anyObf := f["obf_mu"] != "" || f["obf_sigma"] != "" || f["min_pad"] != "" ||
		f["jitter_us"] != "" || f["jitter_ms"] != "" || f["coalesce_ms"] != "" ||
		f["mimic"] != "" || f["filler_min"] != "" || f["filler_ratio"] != "" ||
		f["dir_balance"] != "" || f["rekey"] != "" || f["pow"] != ""
	w("[obfuscation]")
	if f["obf_mu"] != "" {
		w("mu_log = %s", f["obf_mu"])
	}
	if f["obf_sigma"] != "" {
		w("sigma = %s", f["obf_sigma"])
	}
	if f["min_pad"] != "" {
		w("min_pad = %s", f["min_pad"])
		w("max_pad = %s", firstNonEmpty(f["max_pad"], f["min_pad"]))
	}
	if f["jitter_us"] != "" {
		w("jitter_micro_ms = %s", f["jitter_us"])
	}
	if f["jitter_ms"] != "" {
		w("jitter_mean_ms = %s", f["jitter_ms"])
	}
	if f["coalesce_ms"] != "" {
		w("coalesce_ms = %s", f["coalesce_ms"])
	}
	if f["mimic"] != "" {
		w("mimic_tls = %s", f["mimic"])
	}
	if f["filler_min"] != "" {
		w("filler_min = %s", f["filler_min"])
		w("filler_max = %s", firstNonEmpty(f["filler_max"], f["filler_min"]))
	}
	if f["filler_ratio"] != "" {
		w("filler_ratio = %s", f["filler_ratio"])
	}
	if f["dir_balance"] != "" {
		w("dir_balance = %s", f["dir_balance"])
	}
	if f["rekey"] != "" {
		w("rekey_frames = %s", f["rekey"])
	}
	if f["pow"] != "" {
		w("# server requires PoW: difficulty carried in crypto config")
	}
	if !anyObf {
		w("# server defaults")
	}
	w("")
	w("[routing]")
	w("mode = \"global\"")
	_ = cachePath // passed to the core as --serverkey-cache, not via TOML
	return b.String(), nil
}
