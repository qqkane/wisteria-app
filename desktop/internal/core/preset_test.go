package core

import (
	"strings"
	"testing"
)

// The per-profile preset (and the global default fallback) must land in the
// runtime connect.toml as [obfuscation].lvl, which the core expands; an
// invalid label must never reach the wire.
func TestWisteriaTOMLPreset(t *testing.T) {
	mk := func(fields map[string]string) Profile {
		return Profile{
			Type: TWisteria, Host: "example.com", Port: 443,
			ClientPriv: strings.Repeat("A", 43) + "=",
			Fields:     fields,
		}
	}

	// per-profile preset wins
	out, err := WisteriaTOML(mk(map[string]string{"preset": "fast"}), "127.0.0.1:1080", "", true, "stealth")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "lvl = \"fast\"") {
		t.Errorf("per-profile preset must render lvl=fast:\n%s", out)
	}

	// global default fallback when the profile carries none
	out, err = WisteriaTOML(mk(map[string]string{}), "127.0.0.1:1080", "", true, "stealth")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "lvl = \"stealth\"") {
		t.Errorf("global default must render lvl=stealth:\n%s", out)
	}

	// invalid label is dropped, not emitted
	out, err = WisteriaTOML(mk(map[string]string{"preset": "turbo"}), "127.0.0.1:1080", "", true, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "lvl =") {
		t.Errorf("invalid preset must not render lvl:\n%s", out)
	}

	// no preset anywhere = pre-level shape (no lvl key)
	out, err = WisteriaTOML(mk(map[string]string{}), "127.0.0.1:1080", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "lvl =") {
		t.Errorf("absent preset must not render lvl:\n%s", out)
	}
}