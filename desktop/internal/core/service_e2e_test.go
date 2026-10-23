package core

// End-to-end service test against the REAL wisteria cores. It validates the
// generated client TOML against the core's strict config loader and drives a
// full tunnel: server subprocess → profile → Service.Start → SOCKS5 roundtrip.
//
// Skipped unless WIST_E2E=1 with the core paths set, e.g.:
//
//	WIST_E2E=1 WIST_CLIENT=../build/bin/wisteria-client.exe \
//	WIST_SERVER=../build/bin/wisteria-server.exe go test -run TestServiceE2E -v

import (
	"bufio"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/proxy"
)

func TestServiceE2EWisteria(t *testing.T) {
	if os.Getenv("WIST_E2E") != "1" {
		t.Skip("WIST_E2E not set")
	}
	clientBin := os.Getenv("WIST_CLIENT")
	serverBin := os.Getenv("WIST_SERVER")
	for _, p := range []string{clientBin, serverBin} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("core missing: %v", err)
		}
	}

	// Fresh keypair pair (the same X25519 the app generates on import).
	srvPriv, srvPub, err := GenerateX25519()
	if err != nil {
		t.Fatal(err)
	}
	cliPriv, cliPub, err := GenerateX25519()
	if err != nil {
		t.Fatal(err)
	}

	// Free port for the server listener.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srvPort := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	// Server config per configs/server.toml.example (defaults for obfuscation).
	tmp := t.TempDir()
	srvConf := filepath.Join(tmp, "server.toml")
	srvTOML := strings.Join([]string{
		"[server]",
		`listen = "127.0.0.1:` + strconv.Itoa(srvPort) + `"`,
		`transport = "tcp"`,
		"",
		"[crypto]",
		`pattern = "auto"`,
		`static_private = "` + srvPriv + `"`,
		"",
		"[relay]",
		`mode = "direct"`,
		"",
		"[authorized_peers]",
		`"desktop-e2e" = "` + cliPub + `"`,
		"",
	}, "\n")
	if err := os.WriteFile(srvConf, []byte(srvTOML), 0o600); err != nil {
		t.Fatal(err)
	}
	srvCmd := exec.Command(serverBin, "-config", srvConf)
	hideConsole(srvCmd)
	srvStderr, _ := srvCmd.StderrPipe()
	if err := srvCmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = srvCmd.Process.Kill()
		_ = srvCmd.Wait()
	}()
	go func() {
		sc := bufio.NewScanner(srvStderr)
		for sc.Scan() {
			t.Logf("[server] %s", sc.Text())
		}
	}()
	time.Sleep(700 * time.Millisecond) // let the listener bind

	// Echo target the tunnel will reach.
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				io.Copy(c, c)
				c.Close()
			}(c)
		}
	}()

	// The service: isolated dirs, bridge+sysproxy OFF (never touch the host).
	ring := NewLogRing(500)
	ring.SetHook(func(l LogLine) { t.Logf("[%s] %s", l.Tag, l.Text) })
	dirs := Dirs{Data: filepath.Join(tmp, "data"), Runtime: filepath.Join(tmp, "runtime")}
	svc := NewService(dirs, ring, nil)
	svc.SaveSettings(Settings{LocalPort: 21080, HttpPort: 21081, BridgeEnabled: false, SysProxy: false})
	// Silence FindCore: point the settings at the test cores.
	set := svc.Settings()
	set.WisteriaCore = clientBin
	svc.SaveSettings(set)

	profile := Profile{
		Type: TWisteria, Name: "e2e", Host: "127.0.0.1", Port: srvPort,
		ClientPriv: cliPriv,
		Fields:     map[string]string{"pub": srvPub},
	}
	if err := svc.Start(profile); err != nil {
		t.Fatalf("start: %v", err)
	}

	deadline := time.Now().Add(20 * time.Second)
	for {
		st := svc.Status()
		if st.State == StateConnected {
			break
		}
		if st.State == StateDisconnected && st.Error != "" {
			t.Fatalf("connect failed: %s", st.Error)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for connect, state=%s err=%s", st.State, st.Error)
		}
		time.Sleep(150 * time.Millisecond)
	}

	// SOCKS5 roundtrip through the tunnel.
	d, err := proxy.SOCKS5("tcp", "127.0.0.1:21080", nil, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	c, err := d.Dial("tcp", echo.Addr().String())
	if err != nil {
		t.Fatalf("socks dial: %v", err)
	}
	msg := "hello-wisteria-desktop-e2e"
	if _, err := c.Write([]byte(msg)); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(msg))
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(c, got); err != nil {
		t.Fatalf("echo: %v", err)
	}
	if string(got) != msg {
		t.Fatalf("echo mismatch: %q", got)
	}
	c.Close()

	// A second connection exercises the IK (reconnect-cached-key) path's
	// sibling: same session, new yamux stream.
	c2, err := d.Dial("tcp", echo.Addr().String())
	if err != nil {
		t.Fatalf("second dial: %v", err)
	}
	c2.Close()

	svc.Stop()
	deadline = time.Now().Add(10 * time.Second)
	for {
		if svc.Status().State == StateDisconnected {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for disconnect")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
