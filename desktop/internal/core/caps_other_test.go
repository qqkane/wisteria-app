package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTunGrantHintRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; the hint path is unreachable")
	}
	// The hint text must name the exact setcap command.
	_, err := tunGrantHint("/opt/sing-box/sing-box")
	if err == nil {
		t.Fatal("non-root without capability must produce a hint")
	}
	if !strings.Contains(err.Error(), "sudo setcap cap_net_admin=+ep /opt/sing-box/sing-box") {
		t.Fatalf("hint lacks the setcap command: %v", err)
	}
}

func TestBinaryHasCapNetAdmin(t *testing.T) {
	if _, err := exec.LookPath("getcap"); err != nil {
		t.Skip("getcap not installed")
	}
	// A binary we just created carries no file capabilities.
	plain := filepath.Join(t.TempDir(), "plain")
	if err := os.WriteFile(plain, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if binaryHasCapNetAdmin(plain) {
		t.Fatal("freshly created binary must not report cap_net_admin")
	}
}
