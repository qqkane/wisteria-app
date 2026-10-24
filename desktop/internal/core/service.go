package core

// Service: supervises one core process at a time (wisteria-client for the
// native protocol, sing-box for the rest), bridges an HTTP CONNECT proxy to
// the core's SOCKS/mixed inbound, and toggles the Windows system proxy.
// State transitions are reported to the UI through the notify callback and
// the log ring.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type State string

const (
	StateDisconnected State = "disconnected"
	StateConnecting   State = "connecting"
	StateConnected    State = "connected"
)

type Status struct {
	State       State     `json:"state"`
	ProfileID   string    `json:"profile_id,omitempty"`
	ProfileName string    `json:"profile_name,omitempty"`
	Type        string    `json:"type,omitempty"`
	Core        string    `json:"core,omitempty"` // core binary used
	PID         int       `json:"pid,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	SocksAddr   string    `json:"socks_addr,omitempty"`
	HttpAddr    string    `json:"http_addr,omitempty"`
	SysProxy    bool      `json:"sys_proxy"`
	Tun         bool      `json:"tun"` // TUN helper active (all-system traffic)
	Error       string    `json:"error,omitempty"`
}

type Settings struct {
	Language      string `json:"language"` // "ru" | "en"
	Theme         string `json:"theme"`    // ui accent theme id (sunset, neon-dark, ...)
	LocalPort     int    `json:"local_port"`
	HttpPort      int    `json:"http_port"`
	SysProxy      bool   `json:"sys_proxy"`      // set the Windows proxy on connect
	BridgeEnabled bool   `json:"bridge_enabled"` // local HTTP bridge (needed for the system proxy)
	AutoConnect   string `json:"auto_connect"`   // profile id to connect at startup, "" = off
	WisteriaCore  string `json:"wisteria_core"`  // explicit paths (else bin/ next to the exe)
	SingBoxCore   string `json:"singbox_core"`

	// General
	StartMinimized bool `json:"start_minimized"` // hide the window at launch
	ConfirmExit    bool `json:"confirm_exit"`    // ask before quitting while connected
	CheckExtIP     bool `json:"check_ext_ip"`    // resolve the external IP after connect
	AutoPingSec    int  `json:"auto_ping_sec"`   // background TCP ping interval, 0 = off

	// Updates: the zero value keeps the check on (so old configs stay in it).
	NoUpdateCheck bool `json:"no_update_check"`

	// Inbound
	ListenAll bool `json:"listen_all"` // bind proxies on 0.0.0.0 instead of 127.0.0.1

	// Routing / DNS (sing-box profiles only)
	BypassLAN bool   `json:"bypass_lan"` // private ranges go direct
	RemoteDNS string `json:"remote_dns"` // resolver for the outbound server name, "" = system

	// Cores
	MemLimitMB int    `json:"mem_limit_mb"` // GOMEMLIMIT for the core process, 0 = off
	LogLevel   string `json:"log_level"`    // sing-box log level
	MPCore     string `json:"mp_core"`      // legacy default engine; kept for JSON compat
	XrayCore   string `json:"xray_core"`    // explicit xray path (else bin/ next to the exe)

	// Per-protocol engine: "sing-box" | "xray" (hysteria2/tuic are always
	// sing-box, wisteria always the native core).
	CoreSS     string `json:"core_ss"`
	CoreVmess  string `json:"core_vmess"`
	CoreVless  string `json:"core_vless"`
	CoreTrojan string `json:"core_trojan"`

	// Routing (in addition to BypassLAN): lists of domains and IP/CIDRs that
	// bypass the tunnel or are blocked outright, one per line (or separated
	// by commas/semicolons). Empty = no extra rules.
	RouteDirect string `json:"route_direct"`
	RouteBlock  string `json:"route_block"`

	// Inbound: TUN
	TunEnabled     bool   `json:"tun_enabled"`      // route all system traffic via a sing-box tun helper
	TunStack       string `json:"tun_stack"`        // "mixed" (default) | "gvisor" | "system"
	TunStrictRoute bool   `json:"tun_strict_route"` // tighter leak protection, may break LAN access
}

var knownThemes = map[string]bool{
	"lilac": true, "sunset": true, "neon-dark": true, "warm-sand": true,
	"arctic": true, "matrix": true, "mono": true,
}

var knownLogLevels = map[string]bool{
	"trace": true, "debug": true, "info": true, "warn": true, "error": true,
}

var knownEngines = map[string]bool{
	"sing-box": true, "xray": true,
}

func (s *Settings) defaults() {
	if s.Language == "" {
		s.Language = "ru"
	}
	if !knownThemes[s.Theme] {
		s.Theme = "lilac"
	}
	if s.LocalPort == 0 {
		s.LocalPort = 2080
	}
	if s.HttpPort == 0 {
		s.HttpPort = 2081
	}
	if !knownLogLevels[s.LogLevel] {
		s.LogLevel = "warn"
	}
	if s.RemoteDNS != "" && !validDNSAddress(s.RemoteDNS) {
		s.RemoteDNS = ""
	}
	if !knownEngines[s.MPCore] {
		s.MPCore = ""
	}
	// Per-protocol engines default to the legacy mp_core choice (the value
	// older builds saved) and fall back to sing-box.
	for _, p := range []*string{&s.CoreSS, &s.CoreVmess, &s.CoreVless, &s.CoreTrojan} {
		if !knownEngines[*p] {
			*p = s.defaultEngine()
		}
	}
	if s.TunStack != "gvisor" && s.TunStack != "system" {
		s.TunStack = "mixed"
	}
	// The system proxy speaks HTTP CONNECT through the bridge: requesting
	// the proxy without the bridge silently does nothing, so the bridge
	// follows the proxy switch.
	if s.SysProxy {
		s.BridgeEnabled = true
	}
	// Bridge defaults on: the Windows system proxy speaks HTTP CONNECT, not SOCKS.
	if s.LocalPort == s.HttpPort {
		s.HttpPort = s.LocalPort + 1
	}
}

// validDNSAddress accepts the sing-box outbound server address forms.
func validDNSAddress(a string) bool {
	for _, p := range []string{"udp://", "tcp://", "tls://", "https://", "h3://", "quic://"} {
		if strings.HasPrefix(a, p) && len(a) > len(p) {
			return true
		}
	}
	// bare host[:port] — treated as udp by sing-box
	return net.ParseIP(a) != nil || (strings.Contains(a, ".") && !strings.ContainsAny(a, " /\\"))
}

// defaultEngine is the fallback engine for the multiprotocol family when no
// per-protocol choice is set.
func (s *Settings) defaultEngine() string {
	if knownEngines[s.MPCore] {
		return s.MPCore
	}
	return "sing-box"
}

// EngineFor picks the core binary family for a multiprotocol profile:
// ss/vmess/vless/trojan follow the per-protocol settings; hysteria2 and tuic
// are sing-box only. (The native wisteria core is picked earlier.)
func (s *Settings) EngineFor(t ProfileType) string {
	var v string
	switch t {
	case TSS:
		v = s.CoreSS
	case TVMess:
		v = s.CoreVmess
	case TVLESS:
		v = s.CoreVless
	case TTrojan:
		v = s.CoreTrojan
	default:
		return "sing-box"
	}
	if knownEngines[v] {
		return v
	}
	return s.defaultEngine()
}

// ListenHost returns the bind host for local inbounds.
func (s *Settings) ListenHost() string {
	if s.ListenAll {
		return "0.0.0.0"
	}
	return "127.0.0.1"
}

type Dirs struct {
	Data    string // profiles.json, settings.json
	Runtime string // generated core configs
}

type Service struct {
	mu       sync.Mutex
	dirs     Dirs
	settings Settings
	st       Status
	ring     *LogRing
	notify   func() // called on every state change

	cmd       *exec.Cmd
	cancel    context.CancelFunc
	bridge    *HTTPBridge
	proxySet  bool
	tunConf   string   // TUN helper config file, removed on teardown
	wstStatus string   // wisteria --status file path ("" when not a wisteria session)
	wstEvents []WEvent // classified connect-loop feed for the diagnostics page
	sbModern  bool     // cached sing-box >= 1.11 probe
	sbProbed  bool
	teardown  sync.Once
	done      chan struct{} // closed when the session teardown has finished
}

func NewService(dirs Dirs, ring *LogRing, notify func()) *Service {
	if notify == nil {
		notify = func() {}
	}
	return &Service{
		dirs:   dirs,
		ring:   ring,
		notify: notify,
		st:     Status{State: StateDisconnected},
	}
}

func (s *Service) LoadSettings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(filepath.Join(s.dirs.Data, "settings.json"))
	if err == nil {
		_ = jsonUnmarshal(data, &s.settings)
	}
	s.settings.defaults()
	return s.settings
}

func (s *Service) SaveSettings(set Settings) Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	set.defaults()
	s.settings = set
	data, err := jsonMarshalIndent(set)
	if err == nil {
		_ = os.WriteFile(filepath.Join(s.dirs.Data, "settings.json"), data, 0o600)
	}
	return s.settings
}

func (s *Service) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.SocksAddr = fmt.Sprintf("127.0.0.1:%d", s.settings.LocalPort)
	s.st.HttpAddr = fmt.Sprintf("127.0.0.1:%d", s.settings.HttpPort)
	s.st.SysProxy = s.proxySet
	return s.st
}

func (s *Service) logf(format string, args ...any) {
	s.ring.Add("app", fmt.Sprintf(format, args...))
}

// Start connects the profile. A running session is stopped first and its
// teardown awaited, so clicking another server switches tunnels without a
// manual disconnect (and a new core never fights the old one for the port).
func (s *Service) Start(p Profile) error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		s.mu.Lock()
		if s.st.State == StateDisconnected {
			break
		}
		s.mu.Unlock()
		if time.Now().After(deadline) {
			return errors.New("предыдущая сессия не завершилась — попробуйте ещё раз через пару секунд")
		}
		s.Stop()
		select {
		case <-s.sessionDone():
		case <-time.After(9 * time.Second):
		}
	}
	if err := os.MkdirAll(s.dirs.Runtime, 0o700); err != nil {
		s.mu.Unlock()
		return err
	}
	// The SOCKS port must be free before the core starts. A foreign proxy on
	// the same port (v2rayN, another client instance) makes the readiness
	// check below "succeed" against the wrong process — and the wisteria
	// core, which binds SOCKS only after a completed server handshake, then
	// never opens the port at all: the session would sit in CONNECTING for
	// the full 20s readiness deadline. Fail fast with a clear message.
	socksAddr0 := fmt.Sprintf("%s:%d", s.settings.ListenHost(), s.settings.LocalPort)
	if err := checkPortFree(socksAddr0); err != nil {
		s.mu.Unlock()
		return err
	}
	if s.settings.ListenAll {
		s.logf("внимание: прокси слушают на 0.0.0.0 — доступны любой машине в локальной сети без пароля")
	}
	core, args, confFile, err := s.prepareCore(p)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	// CommandContext: cancel() must kill the core. A plain exec.Command
	// would leave Stop() cancelling a context nobody watches.
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, core, args...)
	hideConsole(cmd)
	cmd.Dir = s.dirs.Runtime
	if s.settings.MemLimitMB > 0 {
		// Go's soft memory limit; keeps a misbehaving core from eating the host.
		cmd.Env = append(os.Environ(), fmt.Sprintf("GOMEMLIMIT=%dMiB", s.settings.MemLimitMB))
	}
	stdout, err1 := cmd.StdoutPipe()
	stderr, err2 := cmd.StderrPipe()
	if err1 != nil || err2 != nil {
		cancel()
		s.mu.Unlock()
		return errors.New("std pipes: недоступны")
	}
	if err := cmd.Start(); err != nil {
		cancel()
		s.mu.Unlock()
		return fmt.Errorf("запуск %s: %v", filepath.Base(core), err)
	}
	// Die together: without the job, a hard-killed GUI leaves the core
	// running as an orphan. Best-effort — without the job things still work.
	if err := keepCoreInJob(cmd.Process.Pid); err != nil {
		s.logf("job object: %v (ядро может пережить аварийное завершение GUI)", err)
	}
	s.cmd = cmd
	s.cancel = cancel
	s.teardown = sync.Once{}
	s.done = make(chan struct{})
	s.wstStatus = "" // prepareCore re-sets it for wisteria profiles
	s.wstEvents = nil
	socksAddr := socksAddr0
	httpAddr := fmt.Sprintf("%s:%d", s.settings.ListenHost(), s.settings.HttpPort)
	s.st = Status{
		State:       StateConnecting,
		ProfileID:   p.ID,
		ProfileName: p.Name,
		Type:        string(p.Type),
		Core:        filepath.Base(core),
		PID:         cmd.Process.Pid,
		SocksAddr:   socksAddr,
		HttpAddr:    httpAddr,
	}
	s.logf("запуск %s (pid %d) для %s [%s]", filepath.Base(core), cmd.Process.Pid, p.Endpoint(), p.Type)
	s.mu.Unlock()

	// Core output → log ring (tagged by the binary name so wisteria's own
	// [socks5] prefixes stay readable). Wisteria lines also feed the
	// diagnostics page classifier.
	tag := "core"
	onLine := func(string) {}
	if p.Type == TWisteria {
		onLine = s.noteWisteriaLine
	}
	go pumpPipe(stdout, tag, s.ring, onLine)
	go pumpPipe(stderr, tag, s.ring, onLine)
	go s.supervise(cmd, ctx, cancel, p, confFile, socksAddr, httpAddr)
	s.notify()
	return nil
}

// prepareCore picks the binary and writes the config file. Called with the
// lock held (it only touches settings and disk).
func (s *Service) prepareCore(p Profile) (core string, args []string, confFile string, err error) {
	// User routing lists, parsed once for both renderers.
	directD, directC := parseRouteList(s.settings.RouteDirect)
	blockD, blockC := parseRouteList(s.settings.RouteBlock)
	confFile = filepath.Join(s.dirs.Runtime, "profile-"+p.ID+
		map[bool]string{true: ".toml", false: ".json"}[p.Type == TWisteria])
	if p.Type == TWisteria {
		core = firstNonEmpty(s.settings.WisteriaCore, FindCore("wisteria-client"))
		if core == "" {
			return "", nil, "", fmt.Errorf("ядро wisteria-client не найдено (положите в bin/ рядом с приложением или укажите путь в настройках)")
		}
		tomlText, err := WisteriaTOML(p, fmt.Sprintf("%s:%d", s.settings.ListenHost(), s.settings.LocalPort),
			filepath.Join(s.dirs.Data, "serverkey-"+p.ID+".cache"))
		if err != nil {
			return "", nil, "", err
		}
		if err := os.WriteFile(confFile, []byte(tomlText), 0o600); err != nil {
			return "", nil, "", err
		}
		// Diagnostics feed: the core periodically writes its probe state to
		// disk; the diagnostics page reads it while the session is up.
		statusFile := filepath.Join(s.dirs.Runtime, "wisteria-status.json")
		args = []string{"--config", confFile, "--serverkey-cache",
			filepath.Join(s.dirs.Data, "serverkey-"+p.ID+".cache"),
			"--status", statusFile, "--status-interval", "2"}
		s.wstStatus = statusFile
		return core, args, confFile, nil
	}
	// xray-core as the multiprotocol engine when the per-protocol setting
	// says so; hysteria2/tuic always need sing-box.
	if XraySupports(p.Type) && s.settings.EngineFor(p.Type) == "xray" {
		core = firstNonEmpty(s.settings.XrayCore, FindCore("xray"))
		if core == "" {
			return "", nil, "", fmt.Errorf("xray не найден (скачайте с github.com/XTLS/Xray-core и положите в bin/ или укажите путь в настройках)")
		}
		conf, err := XrayConfig(p, XrayOptions{
			SocksPort:     s.settings.LocalPort,
			Listen:        s.settings.ListenHost(),
			LogLevel:      s.settings.LogLevel,
			BypassLAN:     s.settings.BypassLAN,
			RemoteDNS:     s.settings.RemoteDNS,
			DirectDomains: directD,
			DirectCIDRs:   directC,
			BlockDomains:  blockD,
			BlockCIDRs:    blockC,
		})
		if err != nil {
			return "", nil, "", err
		}
		if err := os.WriteFile(confFile, conf, 0o600); err != nil {
			return "", nil, "", err
		}
		return core, []string{"run", "-c", confFile}, confFile, nil
	}
	core = firstNonEmpty(s.settings.SingBoxCore, FindCore("sing-box"))
	if core == "" {
		return "", nil, "", fmt.Errorf("sing-box не найден (скачайте с github.com/SagerNet/sing-box и положите в bin/ или укажите путь в настройках)")
	}
	conf, err := SingBoxConfig(p, SingBoxOptions{
		MixedPort:     s.settings.LocalPort,
		Listen:        s.settings.ListenHost(),
		LogLevel:      s.settings.LogLevel,
		BypassLAN:     s.settings.BypassLAN,
		RemoteDNS:     s.settings.RemoteDNS,
		DirectDomains: directD,
		DirectCIDRs:   directC,
		BlockDomains:  blockD,
		BlockCIDRs:    blockC,
	})
	if err != nil {
		return "", nil, "", err
	}
	if err := os.WriteFile(confFile, conf, 0o600); err != nil {
		return "", nil, "", err
	}
	return core, []string{"run", "-c", confFile}, confFile, nil
}

func (s *Service) supervise(cmd *exec.Cmd, ctx context.Context, cancel context.CancelFunc, p Profile, confFile, socksAddr, httpAddr string) {
	// Snapshot the settings for the whole session: SaveSettings replaces the
	// struct under the lock, so reading s.settings fields out here would race.
	set := s.Settings()
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	// Readiness: the core's local inbound must accept TCP. A core that dies
	// early loses immediately — its tail log line becomes the error text.
	deadline := time.Now().Add(20 * time.Second)
	ready := false
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case err := <-waitCh:
			s.failSession(cmd, confFile, fmt.Errorf("ядро завершилось до готовности: %v", err))
			return
		default:
		}
		conn, err := net.DialTimeout("tcp", socksAddr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			ready = true
			break
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		s.failSession(cmd, confFile, fmt.Errorf("порт %s так и не открылся (%v)", socksAddr, lastErr))
		return
	}

	// TUN mode: a sing-box helper captures all system traffic into the main
	// core's SOCKS. In this mode the HTTP bridge and the Windows system
	// proxy are pointless — routing happens at the L3 level.
	tunActive := false
	if set.TunEnabled {
		tunWait, err := s.startTun(ctx, p, socksAddr, set)
		if err != nil {
			s.failSession(cmd, confFile, err)
			return
		}
		tunActive = true
		go func() {
			<-tunWait
			if ctx.Err() == nil {
				s.logf("TUN-процесс завершился — остановка сессии")
				s.Stop()
			}
		}()
	}

	// HTTP bridge for the system proxy (Windows proxy settings speak HTTP
	// CONNECT; the cores speak SOCKS/mixed). Bridge failure is not fatal —
	// direct SOCKS users keep working.
	coreName := filepath.Base(cmd.Path)
	s.mu.Lock()
	if tunActive {
		s.logf("TUN активен — системный прокси и HTTP-мост не используются")
	} else {
		if set.BridgeEnabled {
			br, err := StartHTTPBridge(httpAddr, socksAddr)
			if err == nil {
				s.bridge = br
				s.logf("HTTP-мост на http://%s → socks://%s", httpAddr, socksAddr)
			} else {
				s.logf("HTTP-мост не поднялся (%v) — системный прокси работать не будет", err)
			}
		}
		if set.SysProxy && s.bridge != nil {
			if err := SetSystemProxy(true, httpAddr); err == nil {
				s.proxySet = true
				s.logf("системный прокси → %s", httpAddr)
			} else {
				s.logf("системный прокси не установился: %v", err)
			}
		}
	}
	s.st.Tun = tunActive
	if tunActive {
		s.st.Core += " + TUN"
	}
	s.st.State = StateConnected
	s.st.StartedAt = time.Now()
	s.st.Error = ""
	s.mu.Unlock()
	s.logf("подключено: %s [%s] через %s", p.Endpoint(), p.Type, coreName)
	s.notify()

	// Wait for the core to exit (user stop or crash) and tear everything down.
	var waitErr error
	select {
	case waitErr = <-waitCh:
	case <-ctx.Done():
		// Stop() cancelled: the process is being killed; reap it.
		_ = <-waitCh
		waitErr = errors.New("остановлено пользователем")
	}
	s.teardown.Do(func() {
		cancel() // also stops the TUN helper sharing this context
		// The state flips to disconnected immediately (the UI heals and the
		// next Start is unblocked by the done signal below), but the FULL
		// cleanup completes BEFORE that signal: Stop() callers rely on the
		// system proxy reset and the port release having finished. Everything
		// here is bounded — the bridge's Stop waits at most 2s.
		s.mu.Lock()
		bridge := s.bridge
		s.bridge = nil
		proxyWas := s.proxySet
		s.proxySet = false
		tunConf := s.tunConf
		s.tunConf = ""
		s.cancel = nil
		s.cmd = nil
		s.st.Tun = false
		s.st.State = StateDisconnected
		s.st.PID = 0
		s.st.StartedAt = time.Time{}
		s.mu.Unlock()
		if proxyWas {
			if err := SetSystemProxy(false, ""); err != nil {
				s.logf("системный прокси не сбросился: %v", err)
			} else {
				s.logf("системный прокси отключён")
			}
		}
		if bridge != nil {
			bridge.Stop()
		}
		_ = os.Remove(confFile)
		if tunConf != "" {
			_ = os.Remove(tunConf)
		}
		s.mu.Lock()
		if s.done != nil {
			close(s.done)
			s.done = nil
		}
		s.mu.Unlock()
		s.logf("сессия завершена (%v)", waitErr)
		s.notify()
	})
}

// startTun launches the sing-box TUN helper on top of the main core's SOCKS
// inbound and waits for the adapter to appear. Called from supervise with the
// session not yet marked connected; on failure the caller tears the
// whole session down. Reads only the session's settings snapshot.
func (s *Service) startTun(ctx context.Context, p Profile, socksAddr string, set Settings) (<-chan error, error) {
	singbox := firstNonEmpty(set.SingBoxCore, FindCore("sing-box"))
	if singbox == "" {
		return nil, errors.New("для TUN нужен sing-box" + exeSuffix + " (положите в bin/ или укажите путь в настройках)")
	}
	if !IsElevated() {
		if runtime.GOOS == "windows" {
			return nil, errors.New("TUN требует прав администратора — перезапустите приложение от администратора (кнопка в настройках)")
		}
		return nil, errors.New("TUN требует права root — запустите приложение от root (sudo) или выдайте бинарю CAP_NET_ADMIN")
	}
	if runtime.GOOS == "windows" && !hasWintunDLL(singbox, s.dirs) {
		return nil, errors.New("wintun.dll не найден рядом с sing-box.exe — возьмите его из архива релиза sing-box или с wintun.net и положите рядом")
	}
	// The main core's own upstream connection must bypass the tunnel,
	// otherwise it loops back into the TUN.
	ips, err := resolveServerAddrs(p.Host)
	if err != nil {
		return nil, fmt.Errorf("TUN: не удалось определить адрес сервера %s: %v", p.Host, err)
	}
	s.mu.Lock()
	if !s.sbProbed {
		s.sbModern = singBoxModernDNS(singbox)
		s.sbProbed = true
	}
	modern := s.sbModern
	s.mu.Unlock()
	conf, err := TunConfig(TunOptions{
		SocksAddr:   socksAddr,
		ServerIPs:   ips,
		BypassLAN:   set.BypassLAN,
		RemoteDNS:   set.RemoteDNS,
		ModernDNS:   modern,
		LogLevel:    set.LogLevel,
		Stack:       set.TunStack,
		StrictRoute: set.TunStrictRoute,
	})
	if err != nil {
		return nil, err
	}
	confFile := filepath.Join(s.dirs.Runtime, "tun-"+p.ID+".json")
	if err := os.WriteFile(confFile, conf, 0o600); err != nil {
		return nil, err
	}
	tcmd := exec.CommandContext(ctx, singbox, "run", "-c", confFile)
	hideConsole(tcmd)
	tcmd.Dir = s.dirs.Runtime
	tout, e1 := tcmd.StdoutPipe()
	terr, e2 := tcmd.StderrPipe()
	if e1 != nil || e2 != nil {
		return nil, errors.New("tun std pipes: недоступны")
	}
	if err := tcmd.Start(); err != nil {
		return nil, fmt.Errorf("запуск TUN (sing-box): %v", err)
	}
	if err := keepCoreInJob(tcmd.Process.Pid); err != nil {
		s.logf("job object (tun): %v", err)
	}
	go pumpPipe(tout, "tun", s.ring, nil)
	go pumpPipe(terr, "tun", s.ring, nil)
	s.mu.Lock()
	s.tunConf = confFile
	s.mu.Unlock()
	s.logf("TUN: sing-box поверх %s, адрес сервера %s идёт напрямую", socksAddr, strings.Join(ips, ", "))

	waitCh := make(chan error, 1)
	go func() { waitCh <- tcmd.Wait() }()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-waitCh:
			return nil, fmt.Errorf("tun завершился до готовности: %v", err)
		default:
		}
		if _, err := net.InterfaceByName(tunInterfaceName); err == nil {
			return waitCh, nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	_ = tcmd.Process.Kill()
	return nil, errors.New("TUN-адаптер " + tunInterfaceName + " не появился за 15с (см. логи)")
}

// hasWintunDLL: sing-box on Windows needs wintun.dll next to its binary
// (checked on Windows only; the linux build of sing-box carries the tun
// device support natively).
func hasWintunDLL(singbox string, dirs Dirs) bool {
	for _, d := range []string{filepath.Dir(singbox), dirs.Runtime, dirs.Data} {
		if st, err := os.Stat(filepath.Join(d, "wintun.dll")); err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}

// runCoreOutput runs a core binary for its textual output (version probes).
func runCoreOutput(bin string, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	hideConsole(cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// failSession: the core never became ready. Kill it, report, reset. Mirrors
// the supervise teardown (cancel + close(done) after the cleanup), minus the
// bridge/proxy steps that cannot have run yet.
func (s *Service) failSession(cmd *exec.Cmd, confFile string, err error) {
	_ = cmd.Process.Kill()
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.cmd = nil
	// A TUN helper that died before readiness leaves its generated config
	// behind; supervise's teardown path removes the file, this one must too.
	tunConf := s.tunConf
	s.tunConf = ""
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.teardown.Do(func() {
		s.mu.Lock()
		s.st.State = StateDisconnected
		s.st.PID = 0
		s.st.Error = err.Error()
		s.mu.Unlock()
		s.logf("ошибка подключения: %v", err)
		_ = os.Remove(confFile)
		if tunConf != "" {
			_ = os.Remove(tunConf)
		}
		s.mu.Lock()
		if s.done != nil {
			close(s.done)
			s.done = nil
		}
		s.mu.Unlock()
		s.notify()
	})
}

// Stop terminates the current session (idempotent) and waits, bounded, for
// its full teardown. The state flips to disconnected at once (the UI heals
// immediately); the done signal — what Stop waits for — fires only after the
// system proxy reset and the bridge port release have completed, so the next
// connect starts on a clean slate.
func (s *Service) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	state := s.st.State
	done := s.done
	s.mu.Unlock()
	if cancel == nil {
		return
	}
	if state == StateConnected {
		s.logf("отключение...")
	}
	cancel()
	if done != nil {
		select {
		case <-done:
		case <-time.After(8 * time.Second):
			s.logf("остановка сессии затянулась (>8с)")
		}
	}
}

// sessionDone returns the teardown-completion channel of the current
// session (nil when idle).
func (s *Service) sessionDone() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.done
}

// RunningProfile reports the connected/connecting profile id, for delete guards.
func (s *Service) RunningProfile() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.st.State == StateDisconnected {
		return ""
	}
	return s.st.ProfileID
}

func pumpPipe(r interface{ Read([]byte) (int, error) }, tag string, ring *LogRing, onLine func(string)) {
	if r == nil {
		return
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 512*1024)
	for sc.Scan() {
		line := sc.Text()
		ring.Add(tag, line)
		if onLine != nil {
			onLine(line)
		}
	}
}

// noteWisteriaLine classifies a core stdout line into the diagnostics feed.
// Cheap no-op for every non-wisteria session and for lines that carry no
// diagnostic marker.
func (s *Service) noteWisteriaLine(line string) {
	ev, ok := classifyWisteriaLine(line)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	const wstEventCap = 300
	if len(s.wstEvents) >= wstEventCap {
		s.wstEvents = s.wstEvents[1:]
	}
	s.wstEvents = append(s.wstEvents, ev)
}

// WisteriaDiag is the diagnostics page payload: the session state, the
// core's probe rows (--status file) and the classified event feed.
func (s *Service) WisteriaDiag() WisteriaDiag {
	s.mu.Lock()
	st := s.st
	active := s.st.State != StateDisconnected && s.st.Type == string(TWisteria)
	path := s.wstStatus
	events := make([]WEvent, len(s.wstEvents))
	copy(events, s.wstEvents)
	s.mu.Unlock()
	d := WisteriaDiag{Active: active, Status: st, Events: events}
	d.Servers, d.Updated = readWisteriaStatus(path)
	if len(events) > 0 {
		d.LastKind = events[len(events)-1].Kind
	}
	return d
}

type WisteriaDiag struct {
	Active   bool          `json:"active"`
	Status   Status        `json:"status"`
	Servers  []WServerStat `json:"servers"`
	Updated  int64         `json:"updated"` // status-file timestamp, unix nanos
	Events   []WEvent      `json:"events"`
	LastKind string        `json:"last_kind,omitempty"`
}

// checkPortFree reports whether a TCP listener can bind addr right now.
// Go's net.Listen keeps SO_REUSEADDR semantics on Unix, so sockets in
// TIME_WAIT don't trip it — only an active listener (a foreign proxy) does.
func checkPortFree(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("порт %s уже занят другой программой — закройте её (например v2rayN или другой клиент) или смените локальный порт в настройках", addr)
	}
	return ln.Close()
}

// FindCore resolves a core binary: explicit path → bin/ next to the app →
// the app dir → PATH.
func FindCore(name string) string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, cand := range []string{
			filepath.Join(dir, "bin", name+exeSuffix),
			filepath.Join(dir, name+exeSuffix),
		} {
			if st, err := os.Stat(cand); err == nil && !st.IsDir() {
				return cand
			}
		}
	}
	if p, err := exec.LookPath(name + exeSuffix); err == nil {
		return p
	}
	return ""
}
