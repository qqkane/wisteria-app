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
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	subMaxBody      = 4 << 20 // cap the download; lists are kilobytes
	subMaxRedirects = 10
	subUserAgent    = "WisteriaDesktop/1.0"
	subFetchTimeout = 15 * time.Second
)

// DeviceHeaders identifies this client machine to subscription endpoints that
// enforce per-link device limits (HWID).
type DeviceHeaders struct {
	ID   string
	Name string
	OS   string
}

// SubUserinfo holds quota and expiry metadata from the Subscription-Userinfo
// response header (v2rayN / sing-box convention).
type SubUserinfo struct {
	Upload   int64 `json:"upload,omitempty"`
	Download int64 `json:"download,omitempty"`
	Total    int64 `json:"total,omitempty"`
	Expire   int64 `json:"expire,omitempty"`
}

type SubFetch struct {
	Profiles []Profile
	Skipped  int          // lines that did not parse (junk, comments, HTML)
	Userinfo *SubUserinfo // from Subscription-Userinfo header, if present
	Title    string       // from Profile-Title header, if present
}

func validateSubURL(u *url.URL) error {
	if u == nil {
		return errors.New("пустой URL подписки")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("недопустимая схема URL %q (нужен http или https)", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("в URL подписки не указан хост")
	}
	return nil
}

func FetchSubscription(rawURL string, timeout time.Duration) (SubFetch, error) {
	return FetchSubscriptionWithDevice(rawURL, timeout, DeviceHeaders{})
}

func FetchSubscriptionWithDevice(rawURL string, timeout time.Duration, dev DeviceHeaders) (SubFetch, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return SubFetch{}, fmt.Errorf("некорректный URL подписки: %v", err)
	}
	if err := validateSubURL(parsed); err != nil {
		return SubFetch{}, err
	}
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= subMaxRedirects {
				return errors.New("слишком много редиректов")
			}
			return validateSubURL(req.URL)
		},
	}
	req, err := http.NewRequest("GET", parsed.String(), nil)
	if err != nil {
		return SubFetch{}, err
	}
	req.Header.Set("User-Agent", subUserAgent)
	if dev.ID != "" {
		req.Header.Set("X-Device-ID", dev.ID)
	}
	if dev.Name != "" {
		req.Header.Set("X-Device-Name", dev.Name)
	}
	if dev.OS != "" {
		req.Header.Set("X-Device-OS", dev.OS)
	}
	resp, err := client.Do(req)
	if err != nil {
		return SubFetch{}, fmt.Errorf("загрузка: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden {
			msgBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			msg := strings.TrimSpace(string(msgBytes))
			if msg != "" && !strings.HasPrefix(strings.ToLower(msg), "<") {
				return SubFetch{}, fmt.Errorf("доступ запрещён (403): %s", msg)
			}
			return SubFetch{}, errors.New("доступ запрещён (403) — возможно, превышен лимит устройств для этой ссылки")
		}
		return SubFetch{}, fmt.Errorf("сервер ответил %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, subMaxBody+1))
	if err != nil {
		return SubFetch{}, fmt.Errorf("чтение: %v", err)
	}
	if len(body) > subMaxBody {
		return SubFetch{}, errors.New("тело подписки слишком большое (>4 МБ)")
	}
	res, err := ParseSubscriptionBody(string(body))
	if err != nil {
		return res, err
	}
	res.Userinfo = parseSubUserinfo(resp.Header.Get("Subscription-Userinfo"))
	res.Title = parseProfileTitle(resp.Header.Get("Profile-Title"))
	return res, nil
}

// parseSubUserinfo parses "upload=1; download=2; total=3; expire=1700000000".
func parseSubUserinfo(raw string) *SubUserinfo {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var u SubUserinfo
	found := false
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ';' || r == ',' }) {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "upload":
			u.Upload, found = n, true
		case "download":
			u.Download, found = n, true
		case "total":
			u.Total, found = n, true
		case "expire":
			u.Expire, found = n, true
		}
	}
	if !found {
		return nil
	}
	return &u
}

// parseProfileTitle parses "Profile-Title: My sub" or "base64:<encoded>".
func parseProfileTitle(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if len(s) > 7 && strings.EqualFold(s[:7], "base64:") {
		return strings.TrimSpace(b64Flexible(strings.TrimSpace(s[7:])))
	}
	return s
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
