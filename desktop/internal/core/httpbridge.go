package core

// HTTPBridge: a minimal local HTTP proxy that forwards into the core's
// SOCKS/mixed inbound. It exists because the Windows system proxy speaks
// HTTP CONNECT, while the wisteria core speaks only SOCKS5. Handles CONNECT
// (tunnels) and absolute-form plain HTTP via http.Transport round-trip.

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

type HTTPBridge struct {
	ln       net.Listener
	upstream string // host:port of the core's SOCKS inbound
	mu       sync.Mutex
	closed   chan struct{}
	conns    map[net.Conn]struct{}
	wg       sync.WaitGroup
}

func StartHTTPBridge(listenAddr, upstreamSocks string) (*HTTPBridge, error) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", listenAddr, err)
	}
	b := &HTTPBridge{ln: ln, upstream: upstreamSocks, closed: make(chan struct{}),
		conns: map[net.Conn]struct{}{}}
	b.wg.Add(1)
	go b.serve()
	return b, nil
}

// Stop closes the listener and every open client connection, then waits —
// bounded — for the handlers to finish. Wait is bounded because a wedged
// handler must never block the session teardown: Stop runs while the whole
// VPN session is being torn down, and a hang here used to freeze the state
// machine (the session stayed "connected" and no new one could start).
func (b *HTTPBridge) Stop() {
	b.mu.Lock()
	select {
	case <-b.closed:
		b.mu.Unlock()
		return
	default:
	}
	close(b.closed)
	for c := range b.conns {
		_ = c.Close() // unblocks io.Copy inside live tunnels
	}
	ln := b.ln
	b.mu.Unlock()
	_ = ln.Close()

	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

func (b *HTTPBridge) remember(c net.Conn) {
	b.mu.Lock()
	b.conns[c] = struct{}{}
	b.mu.Unlock()
}

func (b *HTTPBridge) forget(c net.Conn) {
	b.mu.Lock()
	delete(b.conns, c)
	b.mu.Unlock()
}

func (b *HTTPBridge) serve() {
	defer b.wg.Done()
	for {
		conn, err := b.ln.Accept()
		if err != nil {
			select {
			case <-b.closed:
				return
			default:
			}
			continue
		}
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			defer b.forget(conn)
			b.remember(conn)
			b.handle(conn)
		}()
	}
}

func (b *HTTPBridge) socksDial() (proxy.Dialer, error) {
	d, err := proxy.SOCKS5("tcp", b.upstream, nil, proxy.Direct)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (b *HTTPBridge) handle(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	dialer, err := b.socksDial()
	if err != nil {
		return
	}

	if req.Method == http.MethodConnect {
		b.tunnel(conn, req.Host, dialer)
		return
	}
	// Plain HTTP in absolute-form (what browsers send to a proxy).
	target := req.URL
	if !target.IsAbs() {
		host := req.Host
		if host == "" {
			return
		}
		u, err := url.Parse("http://" + host + req.RequestURI)
		if err != nil {
			return
		}
		target = u
	}
	outReq := req.Clone(req.Context())
	outReq.URL = target
	outReq.RequestURI = ""
	tr := &http.Transport{}
	if cd, ok := dialer.(proxy.ContextDialer); ok {
		tr.DialContext = cd.DialContext
	}
	resp, err := tr.RoundTrip(outReq)
	if err != nil {
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n"))
		return
	}
	defer resp.Body.Close()
	resp.Write(conn)
}

// tunnel: CONNECT — reply 200, then pipe both ways through one SOCKS conn.
func (b *HTTPBridge) tunnel(conn net.Conn, host string, dialer proxy.Dialer) {
	if host == "" || !strings.Contains(host, ":") {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n"))
		return
	}
	upstream, err := dialer.Dial("tcp", host)
	if err != nil {
		fmt.Fprintf(conn, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: %d\r\n\r\n%s",
			len(err.Error()), err.Error())
		return
	}
	defer upstream.Close()
	conn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	pipeBoth(conn, upstream)
}

func pipeBoth(a, b net.Conn) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		io.Copy(dst, src)
		if tc, ok := dst.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
		done <- struct{}{}
	}
	go cp(a, b)
	go cp(b, a)
	<-done
	<-done
}
