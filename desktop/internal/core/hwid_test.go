package core

import (
	"testing"
)

func TestDeviceIDDeterministic(t *testing.T) {
	dir := t.TempDir()
	id1 := DeviceID(dir)
	id2 := DeviceID(dir)
	if len(id1) != 32 {
		t.Fatalf("expected 32-char hex device id, got %q (len=%d)", id1, len(id1))
	}
	if id1 != id2 {
		t.Fatalf("DeviceID not deterministic: %q != %q", id1, id2)
	}
	for _, c := range id1 {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("non-hex char %q in DeviceID %q", c, id1)
		}
	}
	if DeviceName() == "" {
		t.Fatal("DeviceName returned empty string")
	}
	if DeviceOS() == "" {
		t.Fatal("DeviceOS returned empty string")
	}
}
