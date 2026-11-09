package core

// External IP check through the local proxy. Used after a successful
// connect: the exit address is what the tunnel presents to the internet,
// so it doubles as an end-to-end "the tunnel actually works" probe.

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// ErrTunnelNotReady marks a failed probe whose real cause is a reconnect in
// progress, not a broken tunnel. The SOCKS layer cannot tell them apart.
var ErrTunnelNotReady = errors.New("туннель переподключается")

// transientDialError reports whether a dial failure is the SOCKS-layer
// translation of the core's "wisteria: no live session (connecting)".
// armon/go-socks5 answers every dial error with reply 0x04 (host
// unreachable), and a closed inbound port shows up as connection refused;
// both mean "try again in a moment", not "the tunnel is dead".
func transientDialError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "host unreachable") || strings.Contains(msg, "connection refused")
}

// fetchIPOnce is one probe: GET api.ipify.org and return the body.
func fetchIPOnce(client *http.Client) (string, error) {
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		return "", fmt.Errorf("запрос: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("api.ipify.org: %s", resp.Status)
	}
	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	ip := strings.TrimSpace(string(buf[:n]))
	if ip == "" {
		return "", fmt.Errorf("пустой ответ")
	}
	return ip, nil
}

// FetchExternalIP dials api.ipify.org through the local SOCKS inbound and
// returns the exit address as text.
func FetchExternalIP(socksHost string, socksPort int, timeout time.Duration) (string, error) {
	d, err := proxy.SOCKS5("tcp", fmt.Sprintf("%s:%d", socksHost, socksPort), nil, proxy.Direct)
	if err != nil {
		return "", fmt.Errorf("socks: %w", err)
	}
	cd, ok := d.(proxy.ContextDialer)
	if !ok {
		return "", fmt.Errorf("socks dialer без DialContext")
	}
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:         cd.DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}

	// A reconnect right after connect is the common case: the core has no
	// live session yet, so retry a couple of times before giving up.
	const attempts = 3
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		ip, err := fetchIPOnce(client)
		if err == nil {
			return ip, nil
		}
		lastErr = err
		if attempt == attempts-1 || !transientDialError(err) {
			break
		}
		time.Sleep(1200 * time.Millisecond)
	}
	if transientDialError(lastErr) {
		return "", fmt.Errorf("%w: %v", ErrTunnelNotReady, lastErr)
	}
	return "", lastErr
}
