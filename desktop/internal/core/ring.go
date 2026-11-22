package core

// LogRing: bounded in-memory log with monotonic sequence numbers and 24-hour
// retention. The UI pulls the snapshot on load and then receives live lines
// via events.
//
// A true ring over a fixed backing array: Add at full capacity overwrites
// the oldest slot in O(1). The old append+shift-copy was O(n) per line —
// at sing-box DEBUG (hundreds of lines per second) that meant hundreds of
// 2000-element memmoves per second for nothing.

import (
	"sync"
	"time"
)

// LogRetention is the maximum age of log lines kept in the ring and UI.
const LogRetention = 24 * time.Hour

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
	used int       // number of valid lines currently stored (<= len(buf))
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
	r.addLine(tag, text, time.Now(), true)
}

// AddAt inserts a historical line with an explicit timestamp without firing
// the live hook (used at startup to hydrate the last 24h from app.log).
func (r *LogRing) AddAt(tag, text string, at time.Time) {
	if time.Since(at) > LogRetention {
		return
	}
	r.addLine(tag, text, at, false)
}

func (r *LogRing) addLine(tag, text string, at time.Time, fireHook bool) {
	r.mu.Lock()
	line := LogLine{Seq: r.next, Tag: tag, Text: text, At: at}
	if r.used < len(r.buf) {
		r.buf[(r.head+r.used)%len(r.buf)] = line
		r.used++
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
	if fireHook && hook != nil {
		hook(line)
	}
}

// Clear empties the ring while keeping sequence numbers monotonic.
func (r *LogRing) Clear() {
	r.mu.Lock()
	r.head = 0
	r.used = 0
	r.mu.Unlock()
}

// Snapshot returns lines with seq > since (0 = all) within the 24-hour
// retention window, oldest first.
func (r *LogRing) Snapshot(since int64) []LogLine {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := time.Now().Add(-LogRetention)
	out := make([]LogLine, 0, r.used)
	for i := 0; i < r.used; i++ {
		l := r.buf[(r.head+i)%len(r.buf)]
		if l.Seq > since && !l.At.Before(cutoff) {
			out = append(out, l)
		}
	}
	return out
}
