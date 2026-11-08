package core

// Endpoint latency probes. The probe shape matters: a bare-TCP connect is
// the DPI-visible non-TLS pattern the protocol exists to avoid, so an active
// session is measured through the local SOCKS bridge, TLS transports get a
// real TLS handshake, and only the legacy bare-TCP profiles keep the raw
// dial.

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"golang.org/x/net/proxy"
)

// PingTarget describes one latency probe of a profile endpoint.
type PingTarget struct {
	Host      string
	Port      int
	Transport string // "" | "tcp" | "tls" (the wisteria TLS transport)
	TLSSNI    string // TLS ServerName override; "" = endpoint host
	ViaSocks  string // measure through this local SOCKS5 bridge (host:port); "" = direct probe
}

// PingEndpoint measures the connect latency to the profile's endpoint. With
// ViaSocks set the probe rides the local SOCKS5 bridge (an active session is
// only ever touched through the tunnel); a TLS profile gets a verified TLS
// handshake indistinguishable from a normal client; everything else keeps
// the cheap raw TCP reachability probe.
func PingEndpoint(t PingTarget, timeout time.Duration) (int64, error) {
	addr := joinHostPort(t.Host, t.Port)
	switch {
	case t.ViaSocks != "":
		return pingViaSocks(t.ViaSocks, addr, timeout)
	case t.Transport == "tls":
		return pingTLS(addr, t.TLSSNI, timeout)
	default:
		return pingTCP(addr, timeout)
	}
}

// pingViaSocks measures the SOCKS CONNECT through the local bridge: the
// handshake plus the tunnelled connect to the endpoint, nothing touches the
// server directly.
func pingViaSocks(socksAddr, target string, timeout time.Duration) (int64, error) {
	d, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		return -1, err
	}
	cd, ok := d.(proxy.ContextDialer)
	if !ok {
		// Unreachable with the pinned x/net (its SOCKS5 dialer implements
		// DialContext); the guard keeps this from ever becoming an
		// unbounded dial if the dependency changes under us.
		return -1, errors.New("socks-диалер без поддержки контекста")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	conn, err := cd.DialContext(ctx, "tcp", target)
	if err != nil {
		return -1, err
	}
	_ = conn.Close()
	return time.Since(start).Milliseconds(), nil
}

// pingTLS performs a real TLS handshake against the endpoint: verified
// certificate, ServerName from the profile ("" = endpoint host) — on the
// wire this is an ordinary TLS client.
func pingTLS(addr, sni string, timeout time.Duration) (int64, error) {
	start := time.Now()
	d := &net.Dialer{Timeout: timeout}
	// InsecureSkipVerify stays false on purpose: the probe is a normal TLS
	// client; a lab endpoint with a broken certificate simply reports a
	// failed latency.
	conn, err := tls.DialWithDialer(d, "tcp", addr, &tls.Config{ServerName: sni})
	if err != nil {
		return -1, err
	}
	_ = conn.Close()
	return time.Since(start).Milliseconds(), nil
}

// pingTCP is the legacy bare-TCP reachability probe (non-TLS transports
// only): cheap and good enough to sort a server list.
func pingTCP(addr string, timeout time.Duration) (int64, error) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return -1, err
	}
	_ = conn.Close()
	return time.Since(start).Milliseconds(), nil
}
