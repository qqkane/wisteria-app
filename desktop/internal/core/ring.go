package core

// LogRing: bounded in-memory log with monotonic sequence numbers. The UI
// pulls the snapshot on load and then receives live lines via events.

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
	buf  []LogLine
	next int64
	seq  int64
	hook func(LogLine)
}

func NewLogRing(capacity int) *LogRing {
	if capacity < 16 {
		capacity = 16
	}
	return &LogRing{buf: make([]LogLine, 0, capacity), next: 1}
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
	r.next++
	if len(r.buf) == cap(r.buf) {
		copy(r.buf, r.buf[1:])
		r.buf[len(r.buf)-1] = line
	} else {
		r.buf = append(r.buf, line)
	}
	r.seq = line.Seq
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
	out := make([]LogLine, 0, len(r.buf))
	for _, l := range r.buf {
		if l.Seq > since {
			out = append(out, l)
		}
	}
	return out
}
