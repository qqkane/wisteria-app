package core

// Diagnostics for the sing-box TUN helper. The helper prints the reason it
// refused to start (missing wintun.dll, an admin-only tun ioctl, a config
// field the installed version rejects) to stderr and exits. Without the
// tail those lines only reached the log ring, while the UI error said a
// generic "the adapter did not appear" — the one message that explains
// nothing. lineTail keeps the last few lines so the supervisor can put the
// real error into the status.

import (
	"bufio"
	"regexp"
	"strings"
	"sync"
)

// maxTailLines is how many trailing non-empty lines are kept. sing-box
// prints a short banner before the fatal line, so three would often miss
// the explanation.
const maxTailLines = 5

type lineTail struct {
	mu    sync.Mutex
	lines []string
}

func (t *lineTail) add(line string) {
	line = strings.TrimSpace(stripANSI(line))
	if line == "" {
		return
	}
	t.mu.Lock()
	t.lines = append(t.lines, line)
	if len(t.lines) > maxTailLines {
		t.lines = t.lines[len(t.lines)-maxTailLines:]
	}
	t.mu.Unlock()
}

// best returns the most informative line: the first one that looks like an
// error, else the last one that is not pure decoration.
func (t *lineTail) best() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, l := range t.lines {
		if isErrorLine(l) {
			return l
		}
	}
	for i := len(t.lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(t.lines[i]) != "" {
			return t.lines[i]
		}
	}
	return ""
}

func isErrorLine(line string) bool {
	for _, marker := range []string{"FATAL", "ERROR", "error", "denied", "wintun", "failed"} {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}

// ansiEscape matches the colour sequences sing-box uses when it detects a
// terminal. They are noise in the UI error box.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string { return ansiEscape.ReplaceAllString(s, "") }

// pumpPipeTail is pumpPipe plus the tail: the line still reaches the log
// ring (the log tab must keep showing everything), and the last few lines
// are retained for the startup error message.
func pumpPipeTail(r interface{ Read([]byte) (int, error) }, tag string, ring *LogRing, tail *lineTail) {
	if r == nil {
		return
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 512*1024)
	for sc.Scan() {
		line := sc.Text()
		if tail != nil {
			tail.add(line)
		}
		ring.Add(tag, line)
	}
}
