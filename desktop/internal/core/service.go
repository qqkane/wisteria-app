package core

// Service: supervises one core process at a time (wisteria-client for the
// native protocol, sing-box for the rest), bridges an HTTP CONNECT proxy to
// the core's SOCKS/mixed inbound, and toggles the Windows system proxy.
// State transitions are reported to the UI through the notify callback and
// the log ring.
//
// Session lifecycle (v1.1.7 reconnect rework): every core process lives in
// its own *session struct — process, cancel, teardown Once and done channel
// are session-private. They used to be fields on the Service, and the dying
// goroutine of an old session could consume the new session's teardown Once
// and close its done channel: the next Stop() returned before the old
// teardown had finished, and "Connect" wedged with "предыдущая сессия не
// завершилась" until the app was restarted (the rekey-stuck bug report).
//
// Liveness: the wisteria core survives a dead tunnel — it closes the local
// SOCKS listener and redials with its own backoff. The shell used to watch
// only the process, so the UI sat on CONNECTED while traffic was black
// (the "stuck status" half of the report). Now a watchdog probes the SOCKS
// port while a wisteria session is up; port dead = session dead: the shell
// kills the core and runs its own reconnect loop (exponential backoff), so
// the UI status always equals what the core can actually carry.

import (
	"bufio"
	"context"
	"encoding/json"
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
	State        State     `json:"state"`
	ProfileID    string    `json:"profile_id,omitempty"`
	ProfileName  string    `json:"profile_name,omitempty"`
	Type         string    `json:"type,omitempty"`
	Core         string    `json:"core,omitempty"` // core binary used
	PID          int       `json:"pid,omitempty"`
	StartedAt    time.Time `json:"started_at,omitempty"`
	SocksAddr    string    `json:"socks_addr,omitempty"`
	HttpAddr     string    `json:"http_addr,omitempty"`
	SysProxy     bool      `json:"sys_proxy"`
	Tun          bool      `json:"tun"`          // TUN helper active (all-system traffic)
	Reconnecting bool      `json:"reconnecting"` // session died; auto-reconnect is scheduling
	Attempt      int       `json:"attempt,omitempty"`
	Error        string    `json:"error,omitempty"`
}

type Settings struct {
	Language      string `json:"language"`   // "ru" | "en"
	Theme         string `json:"theme"`      // ui accent theme id (sunset, neon-dark, ...)
	Background    string `json:"background"` // ambient background preset (off, lilac, matrix, gradient)
	LocalPort     int    `json:"local_port"`
	HttpPort      int    `json:"http_port"`
	SysProxy      bool   `json:"sys_proxy"`      // set the Windows proxy on connect
	BridgeEnabled bool   `json:"bridge_enabled"` // local HTTP bridge (needed for the system proxy)
	KillSwitch    bool   `json:"kill_switch"`    // on a non-user teardown, point the system proxy at a blackhole instead of direct (fail closed)
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
	// GitHubToken reads releases of the private app repo (the anonymous API
	// 404s forever there). Plaintext in settings.json (0600) — the same
	// trust level as the client keys in profiles.json; env vars win.
	GitHubToken string `json:"github_token"`

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

	// One-shot marker: engines already migrated to the xray default (v1.1.3).
	EnginesXrayMigration bool `json:"engines_xray_migration"`

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

// knownBackgrounds: ambient background presets. The presets themselves are
// data in the frontend (BGPRESETS in app.js); the shell stores and validates
// the id. "off" is the default (and forced by prefers-reduced-motion).
var knownBackgrounds = map[string]bool{
	"off": true, "lilac": true, "matrix": true, "gradient": true,
}

var knownLogLevels = map[string]bool{
	"trace": true, "debug": true, "info": true, "warn": true, "error": true,
}

var knownEngines = map[string]bool{
	"sing-box": true, "xray": true,
}

// sysProxyBlackhole is the fail-closed system proxy target of the kill
// switch: loopback port 1 is a closed port, so every proxied connection
// fails instantly — traffic is blocked instead of leaking around a dead
// tunnel during the reconnect backoff. The normal set/reset code paths are
// reused, so the override list behaves exactly as with the real bridge.
const sysProxyBlackhole = "127.0.0.1:1"

// sysProxyBlackholePort is the port part of sysProxyBlackhole; the
// stale-proxy cleanup of the next launch must recognize it as our own
// footprint and clear it.
const sysProxyBlackholePort = "1"

// applyNewSettingsDefaults fills settings that ship with a non-zero default
// but are absent in settings.json files written by older builds. A plain
// bool cannot distinguish "field absent" from "explicit false" after the
// unmarshal, so the raw JSON is probed for the key: absent = the user never
// stored a choice → the new default applies; present = the stored value
// wins. Extend the probe (and the first-run save in app.go) with every new
// default-on setting.
func applyNewSettingsDefaults(data []byte, s *Settings) {
	var probe map[string]json.RawMessage
	if jsonUnmarshal(data, &probe) != nil {
		return // corrupt file: the caller replaces the settings anyway
	}
	if _, ok := probe["kill_switch"]; !ok {
		s.KillSwitch = true
	}
}

func (s *Settings) defaults() {
	if s.Language == "" {
		s.Language = "ru"
	}
	if !knownThemes[s.Theme] {
		s.Theme = "lilac"
	}
	if !knownBackgrounds[s.Background] {
		s.Background = "off"
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
	// older builds saved) and fall back to xray.
	for _, p := range []*string{&s.CoreSS, &s.CoreVmess, &s.CoreVless, &s.CoreTrojan} {
		if !knownEngines[*p] {
			*p = s.defaultEngine()
		}
	}
	// One-shot migration (v1.1.3): xray is now the default engine for the
	// multiprotocol family. Settings saved by older builds carry explicit
	// "sing-box" values that were simply the old default — flip them once.
	// A deliberate per-protocol choice made after the migration is never
	// touched again (the marker short-circuits this block).
	if !s.EnginesXrayMigration {
		s.EnginesXrayMigration = true
		for _, p := range []*string{&s.CoreSS, &s.CoreVmess, &s.CoreVless, &s.CoreTrojan} {
			if *p == "" || *p == "sing-box" {
				*p = "xray"
			}
		}
		if !knownEngines[s.MPCore] || s.MPCore == "sing-box" {
			s.MPCore = "xray"
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
// per-protocol choice is set. xray since v1.1.3: better REALITY/XTLS
// coverage and the engine most vless share-links are built for.
func (s *Settings) defaultEngine() string {
	if knownEngines[s.MPCore] {
		return s.MPCore
	}
	return "xray"
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

// session is the whole lifecycle of ONE core process. Every field is owned
// by exactly one session, so a teardown racing with a new Start can never
// touch the new session's state (see the file header).
type session struct {
	profile   Profile
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	done      chan struct{} // closed when the session teardown has finished
	teardown  sync.Once
	confFile  string
	socksAddr string
	httpAddr  string
	bridge    *HTTPBridge
	proxySet  bool
	tunConf   string // TUN helper config file, removed on teardown
}

type Service struct {
	mu       sync.Mutex
	dirs     Dirs
	settings Settings
	st       Status
	ring     *LogRing
	notify   func() // called on every state change

	sess       *session
	wstStatus  string   // wisteria --status file path ("" when not a wisteria session)
	wstEvents  []WEvent // classified connect-loop feed for the diagnostics page
	sbModern   bool     // cached sing-box >= 1.11 probe
	sbProbed   bool
	userStop   bool          // the user asked to disconnect: suppress auto-reconnect
	reconnNext time.Duration // backoff delay for the next auto-reconnect attempt
	startMu    sync.Mutex    // serializes session spawns (UI Connect vs the reconnect loop)

	// Timing knobs (defaults in newService; tests tighten them):
	readinessTimeout time.Duration // core must open its local port within this
	probeInterval    time.Duration // wisteria SOCKS liveness probe period
	probeFailures    int           // consecutive probe failures = session dead
	backoffInit      time.Duration // auto-reconnect initial delay
	backoffMax       time.Duration // auto-reconnect delay ceiling
}

func NewService(dirs Dirs, ring *LogRing, notify func()) *Service {
	if notify == nil {
		notify = func() {}
	}
	return &Service{
		dirs:             dirs,
		ring:             ring,
		notify:           notify,
		st:               Status{State: StateDisconnected},
		readinessTimeout: 20 * time.Second,
		probeInterval:    2 * time.Second,
		probeFailures:    2,
		backoffInit:      time.Second,
		backoffMax:       30 * time.Second,
	}
}

func (s *Service) LoadSettings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.dirs.Data, "settings.json")
	data, err := os.ReadFile(path)
	if err == nil {
		if err := jsonUnmarshal(data, &s.settings); err != nil {
			// Same discipline as profiles.json (store.go): a corrupt file
			// must not be silently overwritten by the next save — set it
			// aside so the user can see why every setting reset.
			s.settings = Settings{}
			backup := path + ".corrupt"
			_ = os.Remove(backup)
			if rerr := os.Rename(path, backup); rerr == nil {
				s.logf("settings.json повреждён (%v) — копия сохранена: %s", err, backup)
			} else {
				s.logf("settings.json повреждён (%v) и копию сохранить не удалось", err)
			}
		} else {
			// Stored choices always win; only keys the file has never heard
			// of pick up their new ship-default.
			applyNewSettingsDefaults(data, &s.settings)
		}
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
	if err != nil {
		return s.settings
	}
	// Temp + rename, same as profiles.json: a plain WriteFile truncated the
	// file in place, so a crash mid-write silently reset every setting
	// (GitHub token, ports, per-protocol engines) to defaults.
	path := filepath.Join(s.dirs.Data, "settings.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return s.settings
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return s.settings
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
	// The real bind host, not a hardcoded 127.0.0.1: with ListenAll the UI
	// must show what is actually listening (0.0.0.0 — LAN-open, warned in
	// the log) instead of a loopback that hides the exposure.
	s.st.SocksAddr = fmt.Sprintf("%s:%d", s.settings.ListenHost(), s.settings.LocalPort)
	s.st.HttpAddr = fmt.Sprintf("%s:%d", s.settings.ListenHost(), s.settings.HttpPort)
	s.st.SysProxy = s.proxySetView()
	return s.st
}

// proxySetView reports whether the CURRENT session has the system proxy on.
// Call with s.mu held.
func (s *Service) proxySetView() bool {
	if s.sess != nil {
		return s.sess.proxySet
	}
	return false
}

func (s *Service) logf(format string, args ...any) {
	s.ring.Add("app", fmt.Sprintf(format, args...))
}

// Start connects the profile. A running session is stopped first and its
// teardown awaited, so clicking another server switches tunnels without a
// manual disconnect (and a new core never fights the old one for the port).
func (s *Service) Start(p Profile) (err error) {
	// The frontend optimistically renders "connecting" before this call, but
	// every early failure below returns with the stored state untouched and
	// no event — the UI then stays wedged on the connecting screen (disabled
	// button included) until a restart. Emit one final state event on the
	// way out; on success this is a no-op (the session notifies on its own).
	defer func() {
		if err != nil {
			s.notify()
		}
	}()
	// Serialize spawns: the UI Connect and the auto-reconnect loop must not
	// race each other's "no session running" check into two cores on one
	// port (the loser dies on checkPortFree and confuses the state machine).
	s.startMu.Lock()
	defer s.startMu.Unlock()
	s.mu.Lock()
	// A fresh Start is a fresh connect intent: clear any user stop and
	// restart the auto-reconnect backoff from the initial delay.
	s.userStop = false
	s.reconnNext = 0
	s.mu.Unlock()
	deadline := time.Now().Add(10 * time.Second)
	for {
		s.mu.Lock()
		sess := s.sess
		s.mu.Unlock()
		if sess == nil {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("предыдущая сессия не завершилась — попробуйте ещё раз через пару секунд")
		}
		s.Stop()
	}
	if err := os.MkdirAll(s.dirs.Runtime, 0o700); err != nil {
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
		return err
	}
	s.mu.Lock()
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
	sess := &session{
		profile:   p,
		cmd:       cmd,
		cancel:    cancel,
		done:      make(chan struct{}),
		confFile:  confFile,
		socksAddr: socksAddr0,
		httpAddr:  fmt.Sprintf("%s:%d", s.settings.ListenHost(), s.settings.HttpPort),
	}
	s.sess = sess
	s.wstStatus = "" // prepareCore re-sets it for wisteria profiles
	s.wstEvents = nil
	s.st = Status{
		State:       StateConnecting,
		ProfileID:   p.ID,
		ProfileName: p.Name,
		Type:        string(p.Type),
		Core:        filepath.Base(core),
		PID:         cmd.Process.Pid,
		SocksAddr:   sess.socksAddr,
		HttpAddr:    sess.httpAddr,
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
	go s.supervise(sess, ctx, cancel)
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
		core = firstNonEmpty(cleanCorePath(s.settings.WisteriaCore), FindCore("wisteria-client"))
		if err := checkCorePath(core); err != nil {
			return "", nil, "", err
		}
		if core == "" {
			return "", nil, "", fmt.Errorf("ядро wisteria-client не найдено — положите его в bin/ рядом с приложением или укажите путь в настройках")
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
		core = firstNonEmpty(cleanCorePath(s.settings.XrayCore), FindCore("xray"))
		if err := checkCorePath(core); err != nil {
			return "", nil, "", err
		}
		if core == "" {
			return "", nil, "", fmt.Errorf("xray не найден — скачайте его кнопкой в Настройки → Ядра или положите в bin/ рядом с приложением")
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
	core = firstNonEmpty(cleanCorePath(s.settings.SingBoxCore), FindCore("sing-box"))
	if err := checkCorePath(core); err != nil {
		return "", nil, "", err
	}
	if core == "" {
		return "", nil, "", fmt.Errorf("sing-box не найден — скачайте его кнопкой в Настройки → Ядра или положите в bin/ рядом с приложением")
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

func (s *Service) supervise(sess *session, ctx context.Context, cancel context.CancelFunc) {
	// Snapshot the settings for the whole session: SaveSettings replaces the
	// struct under the lock, so reading s.settings fields out here would race.
	set := s.Settings()
	p := sess.profile
	socksAddr, httpAddr := sess.socksAddr, sess.httpAddr
	waitCh := make(chan error, 1)
	go func() { waitCh <- sess.cmd.Wait() }()

	// Readiness: the core's local inbound must accept TCP. A core that dies
	// early loses immediately — its tail log line becomes the error text.
	deadline := time.Now().Add(s.readinessTimeout)
	ready := false
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case err := <-waitCh:
			s.failSession(sess, fmt.Errorf("ядро завершилось до готовности: %v", err))
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
		s.failSession(sess, fmt.Errorf("порт %s так и не открылся (%v)", socksAddr, lastErr))
		return
	}

	// TUN mode: a sing-box helper captures all system traffic into the main
	// core's SOCKS. In this mode the HTTP bridge and the Windows system
	// proxy are pointless — routing happens at the L3 level.
	tunActive := false
	if set.TunEnabled {
		tunWait, err := s.startTun(sess, ctx, socksAddr, set)
		if err != nil {
			s.failSession(sess, err)
			return
		}
		tunActive = true
		go func() {
			<-tunWait
			if ctx.Err() == nil {
				s.logf("TUN-процесс завершился — остановка сессии")
				cancel()
			}
		}()
	}

	// HTTP bridge for the system proxy (Windows proxy settings speak HTTP
	// CONNECT; the cores speak SOCKS/mixed). Bridge failure is not fatal —
	// direct SOCKS users keep working.
	coreName := filepath.Base(sess.cmd.Path)
	s.mu.Lock()
	if tunActive {
		s.logf("TUN активен — системный прокси и HTTP-мост не используются")
	} else {
		if set.BridgeEnabled {
			br, err := StartHTTPBridge(httpAddr, socksAddr)
			if err == nil {
				sess.bridge = br
				s.logf("HTTP-мост на http://%s → socks://%s", httpAddr, socksAddr)
			} else {
				s.logf("HTTP-мост не поднялся (%v) — системный прокси работать не будет", err)
			}
		}
		if set.SysProxy && sess.bridge != nil {
			if err := SetSystemProxy(true, httpAddr); err == nil {
				sess.proxySet = true
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
	s.st.Reconnecting = false
	s.st.Attempt = 0
	s.reconnNext = 0 // a live tunnel: the next death restarts backoff from the start
	s.mu.Unlock()
	s.logf("подключено: %s [%s] через %s", p.Endpoint(), p.Type, coreName)
	s.notify()

	// Liveness watchdog (wisteria only): the core closes the local SOCKS
	// listener the moment its yamux session dies (rekey failure, degraded
	// health, server gone) and keeps redialing in its own loop. The process
	// never notices us — only the port does. A dead port for probeFailures
	// consecutive probes = a dead session: kill the core and let the
	// shell-side reconnect loop take over, so the UI stops lying.
	if p.Type == TWisteria {
		go s.watchSession(sess)
	}

	// Wait for the core to exit (user stop, watchdog kill, crash) and tear
	// everything down.
	var waitErr error
	select {
	case waitErr = <-waitCh:
	case <-ctx.Done():
		// Stop() cancelled: the process is being killed; reap it.
		_ = <-waitCh
		waitErr = errors.New("остановлено пользователем")
	}
	s.teardownSession(sess, waitErr)
}

// watchSession probes the wisteria core's SOCKS port while the session is
// up. Returns when the session is gone (teardown closed done) or after the
// watchdog fires its kill.
func (s *Service) watchSession(sess *session) {
	interval, failures := s.probeInterval, s.probeFailures
	fails := 0
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-sess.done:
			return
		case <-t.C:
		}
		conn, err := net.DialTimeout("tcp", sess.socksAddr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			fails = 0
			continue
		}
		fails++
		s.mu.Lock()
		cur := s.sess
		s.mu.Unlock()
		if cur != sess {
			return // the session was already replaced; not ours to police
		}
		if fails < failures {
			continue
		}
		s.logf("wisteria: сессия мертва (порт SOCKS закрыт %v) — перезапускаю ядро, состояние = «переподключение»",
			time.Duration(fails)*interval)
		sess.cancel() // the teardown + auto-reconnect take it from here
		return
	}
}

// teardownSession performs the full session cleanup exactly once, signals
// done, and (unless the user stopped it) hands the profile to the
// auto-reconnect loop.
func (s *Service) teardownSession(sess *session, waitErr error) {
	sess.teardown.Do(func() {
		sess.cancel() // also stops the TUN helper sharing this context
		// The state flips to disconnected immediately (the UI heals and the
		// next Start is unblocked by the done signal below), but the FULL
		// cleanup completes BEFORE that signal: Stop() callers rely on the
		// system proxy reset and the port release having finished. Everything
		// here is bounded — the bridge's Stop waits at most 2s.
		s.mu.Lock()
		bridge := sess.bridge
		sess.bridge = nil
		proxyWas := sess.proxySet
		sess.proxySet = false
		tunConf := sess.tunConf
		sess.tunConf = ""
		if s.sess == sess {
			s.sess = nil
		}
		s.st.Tun = false
		s.st.State = StateDisconnected
		s.st.PID = 0
		s.st.StartedAt = time.Time{}
		auto := !s.userStop
		killSwitch := s.settings.KillSwitch
		s.mu.Unlock()
		if proxyWas {
			// Kill switch: an unexpected death (server gone, core crash,
			// watchdog kill) must not hand the traffic back to the open
			// network while the reconnect backoff runs — point the system
			// proxy at a closed loopback port instead, so everything fails
			// closed. A manual user stop keeps the old behaviour: that is
			// the user asking for direct access. The next successful
			// connect (auto-reconnect included) sets the real proxy again.
			if auto && killSwitch {
				if err := SetSystemProxy(true, sysProxyBlackhole); err != nil {
					s.logf("kill switch: системный прокси не переключён на заглушку: %v", err)
				} else {
					s.logf("kill switch: системный прокси → %s — трафик заблокирован до переподключения", sysProxyBlackhole)
				}
			} else if err := SetSystemProxy(false, ""); err != nil {
				s.logf("системный прокси не сбросился: %v", err)
			} else {
				s.logf("системный прокси отключён")
			}
		}
		if bridge != nil {
			bridge.Stop()
		}
		_ = os.Remove(sess.confFile)
		if tunConf != "" {
			_ = os.Remove(tunConf)
		}
		close(sess.done)
		s.logf("сессия завершена (%v)", waitErr)
		if auto {
			// The tunnel died on its own (server gone, rekey failure, core
			// crash, watchdog kill): keep the user's connect intent alive and
			// relaunch the core with an exponential backoff.
			s.mu.Lock()
			delay := s.reconnNext
			if delay == 0 {
				delay = s.backoffInit
			}
			s.reconnNext = 2 * delay
			if s.reconnNext > s.backoffMax {
				s.reconnNext = s.backoffMax
			}
			s.st.Reconnecting = true
			s.st.Error = fmt.Sprintf("соединение потеряно — автоматическое переподключение (задержка %v)", delay)
			s.mu.Unlock()
			go s.reconnectLoop(sess.profile, delay)
		}
		s.notify()
	})
}

// reconnectLoop relaunches the profile's core with exponential backoff
// until the tunnel is back, the user disconnects, or a newer Start takes
// over. One loop runs at a time: every teardown schedules the next attempt
// with the already-doubled delay stored in s.reconnNext.
func (s *Service) reconnectLoop(p Profile, delay time.Duration) {
	// Interruptible sleep: react to a user Stop / fresh Start within a
	// quarter second, not after the full backoff delay.
	deadline := time.Now().Add(delay)
	for time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
		s.mu.Lock()
		stop := s.userStop
		sess := s.sess
		s.mu.Unlock()
		if stop || sess != nil {
			return
		}
	}
	s.mu.Lock()
	if s.userStop || s.sess != nil {
		s.mu.Unlock()
		return
	}
	attempt := s.st.Attempt + 1
	s.mu.Unlock()
	s.logf("авто-переподключение: попытка %d", attempt)
	if err := s.Start(p); err != nil {
		// Start refused (previous teardown wedged, port taken): the
		// teardown path of the NEXT failure reschedules us.
		s.logf("авто-переподключение: %v", err)
		s.mu.Lock()
		if !s.userStop && s.sess == nil {
			d := s.reconnNext
			s.reconnNext = 2 * d
			if s.reconnNext > s.backoffMax {
				s.reconnNext = s.backoffMax
			}
			go s.reconnectLoop(p, d)
		}
		s.mu.Unlock()
		return
	}
	// Re-annotate the fresh status: Start() replaced the status struct, so
	// stamp the attempt number back on it — the UI must distinguish an
	// auto-reconnect from a first manual connect.
	s.mu.Lock()
	if s.st.State == StateConnecting {
		s.st.Reconnecting = true
		s.st.Attempt = attempt
	}
	s.mu.Unlock()
	s.notify()
	// Start launched the core; supervise() settles the outcome. Connected →
	// the loop's job is done. Failed → failSession's teardown reschedules
	// the next attempt. User stop → userStop stops the chain.
	for {
		time.Sleep(300 * time.Millisecond)
		s.mu.Lock()
		state := s.st.State
		stop := s.userStop
		s.mu.Unlock()
		if stop {
			return
		}
		switch state {
		case StateConnected:
			s.logf("авто-переподключение: соединение восстановлено (попытка %d)", attempt)
			return
		case StateDisconnected:
			return // teardown already scheduled the next attempt
		}
	}
}

// startTun launches the sing-box TUN helper on top of the main core's SOCKS
// inbound and waits for the adapter to appear. Called from supervise with the
// session not yet marked connected; on failure the caller tears the
// whole session down. Reads only the session's settings snapshot.
func (s *Service) startTun(sess *session, ctx context.Context, socksAddr string, set Settings) (<-chan error, error) {
	singbox := firstNonEmpty(cleanCorePath(set.SingBoxCore), FindCore("sing-box"))
	if err := checkCorePath(singbox); err != nil {
		return nil, err
	}
	if singbox == "" {
		return nil, errors.New("для TUN нужен sing-box" + exeSuffix + " (положите в bin/ или укажите путь в настройках)")
	}
	if !IsElevated() {
		if runtime.GOOS == "windows" {
			return nil, errors.New("TUN требует прав администратора — перезапустите приложение от администратора (кнопка в настройках)")
		}
		// Linux: root is not the only route — a file capability on the
		// sing-box binary (setcap cap_net_admin=+ep) grants exactly what
		// the helper needs, without running the whole app elevated.
		if _, err := tunGrantHint(singbox); err != nil {
			return nil, err
		}
	}
	if runtime.GOOS == "windows" && !hasWintunDLL(singbox, s.dirs) {
		return nil, errors.New("wintun.dll не найден рядом с sing-box.exe — возьмите его из архива релиза sing-box или с wintun.net и положите рядом")
	}
	// The main core's own upstream connection must bypass the tunnel,
	// otherwise it loops back into the TUN.
	ips, err := resolveServerAddrs(sess.profile.Host)
	if err != nil {
		return nil, fmt.Errorf("TUN: не удалось определить адрес сервера %s: %v", sess.profile.Host, err)
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
	confFile := filepath.Join(s.dirs.Runtime, "tun-"+sess.profile.ID+".json")
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
	sess.tunConf = confFile // session-owned; the teardown removes the file
	s.logf("TUN: sing-box поверх %s, адрес сервера %s идёт напрямую", socksAddr, strings.Join(ips, ", "))

	waitCh := make(chan error, 1)
	go func() { waitCh <- tcmd.Wait() }()
	tunDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(tunDeadline) {
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
// Bounded: a wedged "core" binary must not hang the settings screen forever
// (the probe runs synchronously inside CoreUpdateCheck).
func runCoreOutput(bin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	hideConsole(cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// failSession: the core never became ready. Kill it, report, reset. Mirrors
// the supervise teardown (cancel + close(done) after the cleanup), minus the
// bridge/proxy steps that cannot have run yet.
func (s *Service) failSession(sess *session, err error) {
	_ = sess.cmd.Process.Kill()
	sess.cancel()
	s.mu.Lock()
	s.st.Error = err.Error()
	s.mu.Unlock()
	s.logf("ошибка подключения: %v", err)
	// teardownSession removes the generated config and (for non-user
	// failures) hands the profile to the auto-reconnect loop. The Error
	// text set above survives into the disconnected state.
	s.teardownSession(sess, err)
}

// Stop terminates the current session (idempotent) and waits, bounded, for
// its full teardown. The state flips to disconnected at once (the UI heals
// immediately); the done signal — what Stop waits for — fires only after the
// system proxy reset and the bridge port release have completed, so the
// next connect starts on a clean slate. Stop is also the user-intent switch:
// it cancels the auto-reconnect chain.
func (s *Service) Stop() {
	s.mu.Lock()
	s.userStop = true
	sess := s.sess
	wasConnected := s.st.State == StateConnected
	s.st.Reconnecting = false
	s.st.Attempt = 0
	s.mu.Unlock()
	if sess == nil {
		s.notify()
		return
	}
	if wasConnected {
		s.logf("отключение...")
	}
	sess.cancel()
	select {
	case <-sess.done:
	case <-time.After(8 * time.Second):
		// Escalate: the core ignored the context cancellation (it cannot
		// ignore SIGKILL, and exec.CommandContext's Cancel already fires
		// one — reaching here means the core process is wedged in a driver).
		s.logf("остановка сессии затянулась (>8с) — убиваю процесс ядра")
		_ = sess.cmd.Process.Kill()
		select {
		case <-sess.done:
		case <-time.After(4 * time.Second):
			s.logf("сессия так и не завершилась — следующее подключение начнётся принудительно")
		}
	}
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

// SetSystemProxyState records a user-made system-proxy toggle on the live
// session, so the UI pill reflects reality (the pill reads the session's
// proxySet flag).
func (s *Service) SetSystemProxyState(on bool) {
	s.mu.Lock()
	if s.sess != nil {
		s.sess.proxySet = on
	}
	s.mu.Unlock()
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

// FindCore resolves a core binary: the writable bin/ (a downloaded copy) →
// bin/ next to the app → the app dir → PATH. On a writable exe dir the
// first two coincide; on an AppImage the downloaded copy lives in the user
// config dir and shadows nothing, while the bundled core is still found in
// the exe-dir candidates.
func FindCore(name string) string {
	for _, cand := range []string{
		filepath.Join(BinDir(), name+exeSuffix),
	} {
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand
		}
	}
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

// cleanCorePath normalizes a user-typed core path. Windows "Copy as path"
// yields a quoted string, and a leading quote makes the path non-absolute
// for exec: it is then resolved against the session's working directory
// (runtime/) and fails as a mangled "runtime\...C:\...: file does not
// exist" instead of a readable "no such file". Stripping the quotes fixes
// the pasted-path case at the source.
func cleanCorePath(p string) string {
	p = strings.TrimSpace(p)
	for len(p) >= 2 {
		q := p[0]
		if (q != '"' && q != '\'') || p[len(p)-1] != q {
			break
		}
		p = strings.TrimSpace(p[1 : len(p)-1])
	}
	return p
}

// checkCorePath gives a broken user-typed core path a readable error at
// prepare time, before exec would fail on it. Bare names (no separators)
// still resolve through PATH at exec time.
func checkCorePath(core string) error {
	if core == "" || !strings.ContainsAny(core, `/\`) {
		return nil
	}
	if _, err := os.Stat(core); err != nil {
		return fmt.Errorf("ядро не найдено по пути из настроек: %s — исправьте его в Настройки → Ядра", core)
	}
	return nil
}
