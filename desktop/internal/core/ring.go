package core

// LogRing: bounded in-memory log with monotonic sequence numbers. The UI
// pulls the snapshot on load and then receives live lines via events.
//
// A true ring over a fixed backing array: Add at full capacity overwrites
// the oldest slot in O(1). The old append+shift-copy was O(n) per line —
// at sing-box DEBUG (hundreds of lines per second) that meant hundreds of
// 2000-element memmoves per second for nothing.

import (
	"sync"
	"time"
)

type LogLine struct {
	Seq  int64     `json:"seq"`
	Tag  string    `json:"tag"` // "app" | "core"
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

type LogRing struct {
	mu   sync.Mutex
	buf  []LogLine // fixed backing array; slot (head+i) holds the i-th oldest line
	head int       // index of the oldest line once the ring is full
	next int64     // sequence number for the next line
	hook func(LogLine)
}

func NewLogRing(capacity int) *LogRing {
	if capacity < 16 {
		capacity = 16
	}
	return &LogRing{buf: make([]LogLine, capacity), next: 1}
}

// SetHook installs a callback fired (outside the lock) for every added line.
func (r *LogRing) SetHook(h func(LogLine)) {
	r.mu.Lock()
	r.hook = h
	r.mu.Unlock()
}

func (r *LogRing) Add(tag, text string) {
	r.mu.Lock()
	line := LogLine{Seq: r.next, Tag: tag, Text: text, At: time.Now()}
	n := r.next - 1 // lines stored so far
	if n < int64(len(r.buf)) {
		r.buf[n] = line
	} else {
		r.buf[r.head] = line
		r.head++
		if r.head == len(r.buf) {
			r.head = 0
		}
	}
	r.next++
	hook := r.hook
	r.mu.Unlock()
	if hook != nil {
		hook(line)
	}
}

// Snapshot returns lines with seq > since (0 = all), oldest first.
func (r *LogRing) Snapshot(since int64) []LogLine {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := r.next - 1
	if n > int64(len(r.buf)) {
		n = int64(len(r.buf))
	}
	out := make([]LogLine, 0, n)
	for i := int64(0); i < n; i++ {
		l := r.buf[(r.head+int(i))%len(r.buf)]
		if l.Seq > since {
			out = append(out, l)
		}
	}
	return out
}
