package core

// The probe strategy: an active session is measured through the local SOCKS
// bridge (fakeSOCKS5 from httpbridge_test.go), a TLS transport gets a real
// verified handshake, and only the legacy bare-TCP transports keep the raw
// dial.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPingEndpointTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	portNum, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}

	ms, err := PingEndpoint(PingTarget{Host: host, Port: portNum}, 2*time.Second)
	if err != nil {
		t.Fatalf("raw probe against a live port: %v", err)
	}
	if ms < 0 {
		t.Fatalf("latency = %d", ms)
	}
	// Legacy transports stay on the raw dial even with a TLS-looking SNI.
	if _, err := PingEndpoint(PingTarget{Host: host, Port: portNum, Transport: "tcp", TLSSNI: "x.example"}, 2*time.Second); err != nil {
		t.Fatalf("legacy tcp transport must keep the raw dial: %v", err)
	}

	if _, err := PingEndpoint(PingTarget{Host: "127.0.0.1", Port: 1}, 300*time.Millisecond); err == nil {
		t.Fatal("raw probe against a dead port must fail")
	}
}

// TestPingEndpointViaSocks: with ViaSocks set the endpoint is only touched
// through the bridge — a successful probe against a dead endpoint proves the
// CONNECT went to the fake SOCKS server, not straight at the target.
func TestPingEndpointViaSocks(t *testing.T) {
	socksLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer socksLn.Close()
	errCh := fakeSOCKS5(t, socksLn)

	ms, err := PingEndpoint(PingTarget{
		Host: "203.0.113.7", Port: 1, // nothing there: only the bridge is real
		ViaSocks: socksLn.Addr().String(),
	}, 2*time.Second)
	if err != nil {
		t.Fatalf("probe through the fake bridge: %v", err)
	}
	if ms < 0 {
		t.Fatalf("latency = %d", ms)
	}
	select {
	case e := <-errCh:
		t.Fatalf("fake socks server: %v", e)
	default:
	}

	// A dead bridge fails the probe (the endpoint is never dialed directly).
	if _, err := PingEndpoint(PingTarget{Host: "127.0.0.1", Port: 1, ViaSocks: "127.0.0.1:1"}, 300*time.Millisecond); err == nil {
		t.Fatal("probe through a dead bridge must fail")
	}
}

// TestPingEndpointTLS: transport=tls must run a real TLS handshake with
// verification on — a self-signed endpoint therefore fails the probe with a
// certificate error, but the handshake itself (the point of the strategy)
// happened on the wire.
func TestPingEndpointTLS(t *testing.T) {
	cert := selfSignedCert(t, "probe.example")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	tlsLn := tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}})
	defer tlsLn.Close()
	go func() {
		for {
			c, err := tlsLn.Accept()
			if err != nil {
				return
			}
			// Hold the server side of the handshake open until the client
			// aborts on verification; then drop it.
			go func(c net.Conn) {
				_ = c.SetDeadline(time.Now().Add(3 * time.Second))
				buf := make([]byte, 64)
				for {
					if _, err := c.Read(buf); err != nil {
						c.Close()
						return
					}
				}
			}(c)
		}
	}()
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	portNum, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}

	_, err = PingEndpoint(PingTarget{Host: host, Port: portNum, Transport: "tls", TLSSNI: "probe.example"}, 2*time.Second)
	if err == nil {
		t.Fatal("a self-signed endpoint must fail the verified probe")
	}
	if !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("want a certificate verification error, got: %v", err)
	}
}

// selfSignedCert mints a throwaway certificate for the given DNS name.
func selfSignedCert(t *testing.T, dns string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: dns},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{dns},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}
