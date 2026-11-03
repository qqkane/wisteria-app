package core

import "testing"

// Once full, the OLDEST line is evicted, not the newest — a regression guard
// for the ring-buffer rewrite of LogRing (the shift-copy version kept order
// by construction; the indexed ring must too). NewLogRing clamps below 16.
func TestLogRingWrapOrder(t *testing.T) {
	r := NewLogRing(16)
	for i := 0; i < 20; i++ {
		r.Add("app", "line")
	}
	got := r.Snapshot(0)
	if len(got) != 16 {
		t.Fatalf("capacity: %d", len(got))
	}
	if got[0].Seq != 5 || got[len(got)-1].Seq != 20 {
		t.Fatalf("oldest must be evicted: seq %d..%d", got[0].Seq, got[len(got)-1].Seq)
	}
}
