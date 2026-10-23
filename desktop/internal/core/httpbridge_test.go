package core

// The bridge is tested against a minimal in-process SOCKS5 server that
// echoes bytes back, proving CONNECT tunnels through the SOCKS upstream.

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// fakeSOCKS5 accepts CONNECT and echoes all payload back to the client.
// Any protocol step that goes wrong is reported on errCh.
func fakeSOCKS5(t *testing.T, ln net.Listener) <-chan error {
	t.Helper()
	errCh := make(chan error, 4)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()
		step := func(n int, err error) bool {
			if err != nil {
				errCh <- fmt.Errorf("step %d: %v", n, err)
				return false
			}
			return true
		}
		// greeting: VER NMETHODS METHODS
		h := make([]byte, 2)
		if !step(1, readFull(conn, h)) {
			return
		}
		methods := make([]byte, h[1])
		if !step(2, readFull(conn, methods)) {
			return
		}
		if _, err := conn.Write([]byte{5, 0}); err != nil { // no auth
			errCh <- err
			return
		}
		// request: VER CMD RSV ATYP ...
		req := make([]byte, 4)
		if !step(3, readFull(conn, req)) {
			return
		}
		if req[1] != 1 { // only CONNECT
			errCh <- fmt.Errorf("cmd %d", req[1])
			return
		}
		switch req[3] {
		case 1: // IPv4
			step(4, readFull(conn, make([]byte, 6)))
		case 3: // domain
			l := make([]byte, 1)
			if !step(4, readFull(conn, l)) {
				return
			}
			step(5, readFull(conn, make([]byte, int(l[0])+2)))
		case 4: // IPv6
			step(4, readFull(conn, make([]byte, 18)))
		default:
			errCh <- fmt.Errorf("atyp %d", req[3])
			return
		}
		if _, err := conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}); err != nil { // success
			errCh <- err
			return
		}
		io.Copy(conn, conn) // echo payload
	}()
	return errCh
}

func readFull(conn net.Conn, buf []byte) error {
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err := io.ReadFull(conn, buf)
	return err
}

func TestHTTPBridgeTunnelsThroughSOCKS(t *testing.T) {
	socksLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer socksLn.Close()
	errCh := fakeSOCKS5(t, socksLn)
	go func() {
		select {
		case e := <-errCh:
			t.Errorf("fake socks server: %v", e)
		case <-time.After(5 * time.Second):
		}
	}()

	br, err := StartHTTPBridge("127.0.0.1:0", socksLn.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer br.Stop()

	req := "GET / HTTP/1.1\r\nHost: echo.example\r\nConnection: close\r\n\r\n"
	// CONNECT through the bridge; the fake upstream echoes the next bytes.
	conn, err := net.Dial("tcp", br.ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Write([]byte("CONNECT echo.example:443 HTTP/1.1\r\nHost: echo.example:443\r\n\r\n"))
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	br1 := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br1, &http.Request{Method: "CONNECT"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("CONNECT status %d", resp.StatusCode)
	}
	_ = req
	conn.Write([]byte("PING"))
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("echo through tunnel: %v", err)
	}
	if string(buf) != "PING" {
		t.Fatalf("got %q", buf)
	}
}

// Regression: Stop used to block forever when a client held a CONNECT
// tunnel open — the browser side never closes, so pipeBoth never finished
// and the session teardown froze with the state stuck on "connected".
// Stop must force-close open tunnels and return promptly.
func TestHTTPBridgeStopWithOpenTunnel(t *testing.T) {
	socksLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer socksLn.Close()
	// A SOCKS5 upstream that completes the handshake, then holds the
	// connection silently — an idle browser tunnel through the system proxy.
	go func() {
		for {
			c, err := socksLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				h := make([]byte, 2)
				if _, err := io.ReadFull(c, h); err != nil {
					return
				}
				if _, err := c.Write([]byte{5, 0}); err != nil { // no auth
					return
				}
				hdr := make([]byte, 4) // VER CMD RSV ATYP
				if _, err := io.ReadFull(c, hdr); err != nil {
					return
				}
				switch hdr[3] {
				case 1: // IPv4
					if _, err := io.ReadFull(c, make([]byte, 6)); err != nil {
						return
					}
				case 4: // IPv6
					if _, err := io.ReadFull(c, make([]byte, 18)); err != nil {
						return
					}
				default: // domain
					l := make([]byte, 1)
					if _, err := io.ReadFull(c, l); err != nil {
						return
					}
					if _, err := io.ReadFull(c, make([]byte, int(l[0])+2)); err != nil {
						return
					}
				}
				if _, err := c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
					return
				}
				io.Copy(io.Discard, c)
			}(c)
		}
	}()

	br, err := StartHTTPBridge("127.0.0.1:0", socksLn.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", br.ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("CONNECT idle.example:443 HTTP/1.1\r\nHost: idle.example:443\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("CONNECT status %d", resp.StatusCode)
	}

	done := make(chan struct{})
	go func() {
		br.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop did not return within 3s while a tunnel was open")
	}
}

func TestParseManualWisteriaFieldsRoundTrip(t *testing.T) {
	// The GUI manual form edits Fields directly; render must accept it.
	p := Profile{ID: "m", Type: TWisteria, Host: "h", Port: 1, ClientPriv: "K==",
		Fields: map[string]string{}}
	out, err := WisteriaTOML(p, "127.0.0.1:2080", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("empty config")
	}
}
