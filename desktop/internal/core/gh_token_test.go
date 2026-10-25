package core

import "testing"

func TestGhTokenPrecedence(t *testing.T) {
	// No sources → empty.
	t.Setenv("WISTERIA_GH_TOKEN", "")
	t.Setenv("WISTERIA_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	SetGitHubToken("")
	if ghToken() != "" {
		t.Fatal("expected empty token with no sources")
	}

	// Settings override alone.
	SetGitHubToken("tok-from-settings")
	if ghToken() != "tok-from-settings" {
		t.Fatalf("settings token ignored: %q", ghToken())
	}

	// Env wins over the settings override.
	t.Setenv("GITHUB_TOKEN", "tok-from-env")
	if ghToken() != "tok-from-env" {
		t.Fatalf("env token must win: %q", ghToken())
	}
	t.Setenv("WISTERIA_GH_TOKEN", "tok-wisteria")
	if ghToken() != "tok-wisteria" {
		t.Fatalf("WISTERIA_GH_TOKEN must have top precedence: %q", ghToken())
	}

	// Settings value is trimmed.
	SetGitHubToken("  padded  ")
	t.Setenv("WISTERIA_GH_TOKEN", "")
	t.Setenv("WISTERIA_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	if ghToken() != "padded" {
		t.Fatalf("settings token not trimmed: %q", ghToken())
	}
	SetGitHubToken("")
}
