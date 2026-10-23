package core

// Subscription fetching. Providers serve either a base64 blob of newline
// separated share links or the plain list itself; both are parsed with
// ParseLink, so every scheme the app understands works inside a
// subscription (wisteria, vless, vmess, ss, trojan, hysteria2, tuic).

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	subMaxBody      = 4 << 20 // cap the download; lists are kilobytes
	subUserAgent    = "WisteriaDesktop/0.5"
	subFetchTimeout = 15 * time.Second
)

type SubFetch struct {
	Profiles []Profile
	Skipped  int // lines that did not parse (junk, comments, HTML)
}

func FetchSubscription(rawURL string, timeout time.Duration) (SubFetch, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return SubFetch{}, err
	}
	req.Header.Set("User-Agent", subUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return SubFetch{}, fmt.Errorf("загрузка: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return SubFetch{}, fmt.Errorf("сервер ответил %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, subMaxBody+1))
	if err != nil {
		return SubFetch{}, fmt.Errorf("чтение: %v", err)
	}
	if len(body) > subMaxBody {
		return SubFetch{}, errors.New("тело подписки слишком большое (>4 МБ)")
	}
	return ParseSubscriptionBody(string(body))
}

// ParseSubscriptionBody splits the body into share links. Convenience for
// tests and for importing a pasted list.
func ParseSubscriptionBody(body string) (SubFetch, error) {
	text := decodeSubBody(body)
	var res SubFetch
	sawHTML := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		low := strings.ToLower(line)
		if strings.HasPrefix(low, "<!doctype") || strings.HasPrefix(low, "<html") {
			sawHTML = true
			continue
		}
		profs, err := ParseLinkAll(line) // bundles expand to one profile per server
		if err != nil {
			res.Skipped++
			continue
		}
		res.Profiles = append(res.Profiles, profs...)
	}
	if len(res.Profiles) == 0 {
		if sawHTML {
			return res, errors.New("подписка вернула HTML-страницу, а не список ссылок")
		}
		return res, fmt.Errorf("в подписке нет ни одной распознанной ссылки (пропущено строк: %d)", res.Skipped)
	}
	return res, nil
}

// decodeSubBody unwraps the conventional base64 wrapper. A decoded body
// counts only if it actually looks like a link list ("://"); plain-text
// lists (and anything non-base64) fall through unchanged.
func decodeSubBody(body string) string {
	compact := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, body)
	if dec := b64Flexible(compact); dec != "" && strings.Contains(dec, "://") {
		return dec
	}
	return body
}
