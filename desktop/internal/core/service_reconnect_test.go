package core

// Reconnect regression tests (the rekey-stuck bug, v1.1.7).
//
// The production cores need keys and a full handshake, which makes the
// reconnect state machine hard to exercise deterministically. These tests
// drive Service against a FAKE core: a compiled helper (testdata/fakecore)
// that mimics the wisteria core's observable contract — it binds the local
// SOCKS port while the session is up, closes the port on session death and
// stays alive (that is what wedged the real shell: the process never exits,
// so the old process-only supervisor sat on CONNECTED forever).
//
// Fake modes via env:
//   - FAKECORE_MODE=bind_forever (default): binds and serves
//   - FAKECORE_MODE=close_after: binds, closes the listener after
//     FAKECORE_CLOSE_AFTER seconds, process keeps running
//   - FAKECORE_MODE=no_bind: runs forever without ever binding (dead server)
//   - FAKECORE_MODE=exit_immediate: exits at once (core crash)
// FAKECORE_PIDFILE appends one line per spawn — tests count core launches.

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

var fakeCorePath string
var fakeCoreBuildErr error

func TestMain(m *testing.M) {
	// Build the fake core once. No toolchain? The reconnect tests self-skip.
	tmp, err := os.MkdirTemp("", "wisteria-fakecore")
	if err == nil {
		fakeCorePath = filepath.Join(tmp, "fakecore")
		if runtime.GOOS == "windows" {
			fakeCorePath += ".exe" // CreateProcess refuses to exec an extension-less binary
		}
		gobin := filepath.Join(runtime.GOROOT(), "bin", "go")
		if _, serr := os.Stat(gobin); serr != nil {
			gobin = "go"
		}
		build := exec.Command(gobin, "build", "-o", fakeCorePath, "./testdata/fakecore")
		if out, berr := build.CombinedOutput(); berr != nil {
			fakeCoreBuildErr = fmt.Errorf("%v: %s", berr, out)
		}
	}
	code := m.Run()
	if fakeCorePath != "" {
		_ = os.RemoveAll(filepath.Dir(fakeCorePath))
	}
	os.Exit(code)
}

// fakeService wires a Service to the fake core with lab-fast timings.
// Returns the service, the SOCKS address, and a pidfile reader.
func fakeService(t *testing.T, mode string, closeAfter string) (*Service, string, func() int) {
	t.Helper()
	if fakeCoreBuildErr != nil {
		t.Skipf("fake core build failed: %v", fakeCoreBuildErr)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	socksAddr := fmt.Sprintf("127.0.0.1:%d", port)

	tmp := t.TempDir()
	pidfile := filepath.Join(tmp, "pids")
	t.Setenv("FAKECORE_MODE", mode)
	t.Setenv("FAKECORE_PORT", strconv.Itoa(port))
	t.Setenv("FAKECORE_PIDFILE", pidfile)
	t.Setenv("FAKECORE_CLOSE_AFTER", closeAfter)

	ring := NewLogRing(1000)
	ring.SetHook(func(l LogLine) { t.Logf("[%s] %s", l.Tag, l.Text) })
	dirs := Dirs{Data: filepath.Join(tmp, "data"), Runtime: filepath.Join(tmp, "runtime")}
	svc := NewService(dirs, ring, nil)
	svc.readinessTimeout = 1500 * time.Millisecond
	svc.probeInterval = 300 * time.Millisecond
	svc.probeFailures = 2
	svc.backoffInit = 300 * time.Millisecond
	svc.backoffMax = time.Second
	svc.SaveSettings(Settings{LocalPort: port, HttpPort: port + 1, BridgeEnabled: false, SysProxy: false})
	set := svc.Settings()
	set.WisteriaCore = fakeCorePath
	svc.SaveSettings(set)
	t.Cleanup(svc.Stop)

	spawns := func() int {
		data, err := os.ReadFile(pidfile)
		if err != nil {
			return 0
		}
		return len(strings.Fields(string(data)))
	}
	return svc, socksAddr, spawns
}

func fakeProfile() Profile {
	return Profile{
		Type: TWisteria, ID: "fake1", Name: "fake",
		Host: "127.0.0.1", Port: 1, ClientPriv: "AA==",
	}
}

// waitState polls until the service reaches want (or the timeout fires).
// Returns the final status.
func waitState(t *testing.T, svc *Service, want State, timeout time.Duration) Status {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		st := svc.Status()
		if st.State == want {
			return st
		}
		if time.Now().After(deadline) {
			return st
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func socksAlive(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 400*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// TestWatchdogAutoReconnect is the exact rekey-stuck scenario: the core
// process survives the session death (SOCKS port closed, traffic dead). The
// shell must (a) notice within the probe window, (b) flip to an honest
// connecting/reconnecting state, (c) relaunch the core on its own, and
// (d) land back at connected — all without a manual click, let alone a
// restart. Then the user's Disconnect must stop the chain for good.
func TestWatchdogAutoReconnect(t *testing.T) {
	svc, addr, spawns := fakeService(t, "close_after", "1.0")
	if err := svc.Start(fakeProfile()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if st := waitState(t, svc, StateConnected, 5*time.Second); st.State != StateConnected {
		t.Fatalf("never reached connected: %+v", st)
	}

	// Session death: the fake closes the port at t+1s and stays alive.
	// The watchdog (2 probes x 300ms) must first LEAVE the connected state
	// — that is the "status equals core truth" half of the fix —
	watchdogFired := false
	probeDeadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(probeDeadline) {
		if svc.Status().State != StateConnected {
			watchdogFired = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !watchdogFired {
		t.Fatalf("watchdog never flipped the status off CONNECTED after the port died: %+v", svc.Status())
	}
	// …and the auto-reconnect (backoff 300ms + relaunch) must land back at
	// connected well inside 10s, on a serving port, with a relaunched core.
	st := waitState(t, svc, StateConnected, 10*time.Second)
	if st.State != StateConnected {
		t.Fatalf("auto-reconnect did not restore the tunnel: %+v (spawns=%d)", st, spawns())
	}
	if !socksAlive(addr) {
		t.Fatal("socks port dead after auto-reconnect")
	}
	if n := spawns(); n < 2 {
		t.Fatalf("core was not relaunched: spawns=%d", n)
	}

	// User stop: the chain must not resurrect the session.
	svc.Stop()
	if st := svc.Status(); st.State != StateDisconnected || st.Reconnecting {
		t.Fatalf("after Stop: want disconnected/reconnecting=false, got %+v", st)
	}
	before := spawns()
	time.Sleep(2500 * time.Millisecond) // longer than backoffMax
	if after := spawns(); after != before {
		t.Fatalf("auto-reconnect kept spawning after user Stop: %d → %d", before, after)
	}
}

// TestManualConnectFromStuckState: the second half of the bug report — the
// «Подключить» button must work from ANY state, including the stuck
// connected-with-dead-port one. A manual Start during the outage window
// (before the watchdog fires) must win: kill the old core, start a new one,
// land connected.
func TestManualConnectFromStuckState(t *testing.T) {
	svc, addr, spawns := fakeService(t, "close_after", "1.0")
	if err := svc.Start(fakeProfile()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if st := waitState(t, svc, StateConnected, 5*time.Second); st.State != StateConnected {
		t.Fatalf("never reached connected: %+v", st)
	}
	// Kill the port now; the app still believes it is connected.
	time.Sleep(1100 * time.Millisecond)
	if socksAlive(addr) {
		t.Skip("fake did not close the port in time")
	}
	if st := svc.Status(); st.State != StateConnected {
		t.Logf("watchdog already fired: %+v", st) // racing the watchdog is fine
	}
	// The manual click (App.Connect → Service.Start) from the stuck state.
	if err := svc.Start(fakeProfile()); err != nil {
		t.Fatalf("manual Start from stuck state failed: %v", err)
	}
	st := waitState(t, svc, StateConnected, 6*time.Second)
	if st.State != StateConnected || !socksAlive(addr) {
		t.Fatalf("manual Start did not land connected: %+v (spawns=%d)", st, spawns())
	}
}

// TestAutoReconnectBackoffOnDeadServer: the core never binds (dead server).
// The shell must keep the connect intent alive with a visible reconnecting
// state and a GROWING delay, and a user Stop must silence it completely.
func TestAutoReconnectBackoffOnDeadServer(t *testing.T) {
	svc, _, spawns := fakeService(t, "no_bind", "")
	if err := svc.Start(fakeProfile()); err != nil {
		t.Fatalf("start: %v", err)
	}
	// The first attempt burns the readiness window, then the loop chains.
	deadline := time.Now().Add(12 * time.Second)
	sawReconnecting, attempts := false, 0
	for time.Now().Before(deadline) {
		st := svc.Status()
		if st.Reconnecting {
			sawReconnecting = true
		}
		if st.Attempt > attempts {
			attempts = st.Attempt
		}
		if attempts >= 3 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !sawReconnecting || attempts < 3 {
		t.Fatalf("want reconnecting with attempt>=3, got reconnecting=%v attempt=%d", sawReconnecting, attempts)
	}

	// User Stop cancels the chain: no more core launches after a grace
	// window, state disconnected, reconnecting cleared.
	svc.Stop()
	time.Sleep(2 * time.Second)
	st := svc.Status()
	if st.State != StateDisconnected || st.Reconnecting {
		t.Fatalf("after Stop: %+v", st)
	}
	before := spawns()
	time.Sleep(2500 * time.Millisecond)
	if after := spawns(); after != before {
		t.Fatalf("cores kept spawning after Stop: %d → %d", before, after)
	}
}

// TestStartStopStorm is the v1.1.6 race regression: with the per-Service
// teardown Once and done channel, Start→(core dies)→Start→Stop storms let
// the dying goroutine of session N consume the Once/close the done channel
// of session N+1 — "предыдущая сессия не завершилась" until restart. Every
// Start here must land in a real state, and nothing may panic.
func TestStartStopStorm(t *testing.T) {
	svc, _, spawns := fakeService(t, "exit_immediate", "")
	for i := 0; i < 5; i++ {
		if err := svc.Start(fakeProfile()); err != nil {
			t.Fatalf("storm round %d: %v", i, err)
		}
		// Don't wait for readiness to expire — hammer Start immediately.
		if err := svc.Start(fakeProfile()); err != nil {
			t.Fatalf("storm round %d (re-Start): %v", i, err)
		}
	}
	svc.Stop()
	if st := svc.Status(); st.State != StateDisconnected {
		t.Fatalf("after storm Stop: %+v", st)
	}
	before := spawns()
	time.Sleep(1500 * time.Millisecond)
	if after := spawns(); after != before {
		t.Fatalf("cores kept spawning after Stop: %d → %d", before, after)
	}
}

// TestStatusEqualsCoreTruth is a compact UI-contract check: during the
// outage window the status must not claim connected. Watchdog (2×300ms
// probes) fires after the fake closes the port at 1s — by t+2s the shell
// must be off CONNECTED and on the reconnect path.
func TestStatusEqualsCoreTruth(t *testing.T) {
	svc, _, _ := fakeService(t, "close_after", "1.0")
	if err := svc.Start(fakeProfile()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if st := waitState(t, svc, StateConnected, 5*time.Second); st.State != StateConnected {
		t.Fatalf("never reached connected: %+v", st)
	}
	time.Sleep(1800 * time.Millisecond) // port died at 1.0s, watchdog by ~1.8s
	st := svc.Status()
	if st.State == StateConnected && !socksAlive(st.SocksAddr) {
		// The only legal CONNECTED claim is a serving port.
		t.Fatalf("UI would lie: status CONNECTED while SOCKS %s is dead", st.SocksAddr)
	}
}
