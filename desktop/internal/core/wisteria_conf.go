package core

// Renders a full wisteria client TOML from a profile. Field names match
// configs/client.toml.example; the core validates on load, so a wrong value
// surfaces in the core log rather than silently degrading the tunnel.

import (
	"fmt"
	"strings"
)

// WisteriaTOML renders the client config. socksListen is the local SOCKS5
// address; cachePath is the per-profile server-key cache (XX→IK pinning).
func WisteriaTOML(p Profile, socksListen, cachePath string) (string, error) {
	if p.Type != TWisteria {
		return "", fmt.Errorf("профиль %q не wisteria", p.ID)
	}
	if p.ClientPriv == "" {
		return "", fmt.Errorf("у профиля нет client key (static_private)")
	}
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	w("[client]")
	w("socks5_listen = %q", socksListen)
	w("")
	// The loader requires the legacy [server] endpoint even when the pool is
	// given via [[servers]] (it validates before wrapping single entries).
	w("[server]")
	w("endpoint  = %q", p.Endpoint())
	w("transport = \"tcp\"")
	w("")
	if pub := p.Fields["pub"]; pub != "" {
		// A pinned key goes into the [[servers]] row (per-endpoint pin, IK
		// from the first connect) and into crypto.server_static_public.
		w("[[servers]]")
		w("endpoint   = %q", p.Endpoint())
		w("weight     = 1")
		w("public_key = %q", pub)
		w("")
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
