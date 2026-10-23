package core

// Wisteria diagnostics feed: the native core (a) writes a per-server probe
// status JSON when started with --status, and (b) logs its connect loop to
// stdout ("CONNECTED via", "cannot reach", "handshake refused", "retrying
// in", "health: ..."). The service turns both into a small, classified feed
// so the UI can answer "why is there no connection" — network down, server
// blocked/unauthorized, retrying, etc.

import (
	"encoding/json"
	"os"
	"strings"
	"time"
)

// WServerStat is one row of the core's probe status file (its ServerStatus
// marshals without tags, so the JSON keys are the Go field names).
type WServerStat struct {
	Endpoint  string `json:"endpoint"`
	RTTMs     int64  `json:"rtt_ms"`
	Reachable bool   `json:"reachable"`
	Failures  int64  `json:"failures"`
}

type wStatusFile struct {
	Updated int64 `json:"updated"`
	Servers []struct {
		Endpoint  string `json:"Endpoint"`
		Weight    int    `json:"Weight"`
		PublicKey string `json:"PublicKey"`
		RTTMs     int64  `json:"RTTMs"`
		Reachable bool   `json:"Reachable"`
		Failures  int64  `json:"Failures"`
	} `json:"servers"`
}

// WEvent is one classified line of the core's connect loop.
type WEvent struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"` // up | down | blocked | nonet | retry | health | info
	Text string    `json:"text"` // human-readable (RU)
	Raw  string    `json:"raw"`  // the core's original line
}

// classifyWisteriaLine maps a core log line onto a diagnostics event.
// Order matters: specific markers first. Hint lines (the core indents them
// with two spaces) are skipped — they follow their parent event.
func classifyWisteriaLine(line string) (WEvent, bool) {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(line, "  ") {
		return WEvent{}, false
	}
	ev := WEvent{Raw: t}
	switch {
	case strings.Contains(t, "CONNECTED via"):
		ev.Kind = "up"
		ev.Text = "Подключено — туннель работает"
	case strings.Contains(t, "handshake refused"):
		ev.Kind = "blocked"
		ev.Text = "Сервер отверг рукопожатие: ссылка устарела или ключ не добавлен в authorized_peers (при живой сети это блокировка/отзыв доступа)"
	case strings.Contains(t, "cannot reach "):
		ev.Kind = "nonet"
		ev.Text = "Сервер недоступен: TCP-соединение не прошло — упал интернет, сервер выключен или провайдер режет доступ"
	case strings.Contains(t, "session to") && strings.Contains(t, "ended"):
		ev.Kind = "down"
		ev.Text = "Сессия прервана — ядро переподключается"
	case strings.Contains(t, "retrying in"):
		ev.Kind = "retry"
		ev.Text = "Переподключение: " + strings.TrimSuffix(t, " - press d to stop, c to retry right now")
	case strings.Contains(t, "health: rtt="):
		ev.Kind = "health"
		ev.Text = "Состояние канала: " + strings.TrimPrefix(t, "health: ")
	case strings.Contains(t, "client key in the config"):
		ev.Kind = "info"
		ev.Text = "Ошибка конфигурации: неверный клиентский ключ"
	default:
		return WEvent{}, false
	}
	ev.At = time.Now()
	return ev, true
}

// readWisteriaStatus parses the core's --status file; a missing or young
// file (the first tick is status-interval seconds after start) is not an
// error — the page simply shows no probe rows yet.
func readWisteriaStatus(path string) ([]WServerStat, int64) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0
	}
	var f wStatusFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, 0
	}
	out := make([]WServerStat, 0, len(f.Servers))
	for _, s := range f.Servers {
		out = append(out, WServerStat{Endpoint: s.Endpoint, RTTMs: s.RTTMs, Reachable: s.Reachable, Failures: s.Failures})
	}
	return out, f.Updated
}
