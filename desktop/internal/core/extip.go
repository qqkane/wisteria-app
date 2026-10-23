package core

// External IP check through the local proxy. Used after a successful
// connect: the exit address is what the tunnel presents to the internet,
// so it doubles as an end-to-end "the tunnel actually works" probe.

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

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
