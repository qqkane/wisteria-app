package main

// App: the API surface bound to the frontend (window.go.main.App.*).
// Thin on purpose — the logic lives in desktop/internal/core.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/qqkane/wisteria-app/desktop/internal/core"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const appVersion = "1.1.12"

type App struct {
	ctx      context.Context
	svc      *core.Service
	store    *core.Store
	ring     *core.LogRing
	dirs     core.Dirs
	pingStop chan struct{}
	pingMu   sync.Mutex
}

func NewApp() *App { return &App{} }

func dataDirs() (core.Dirs, error) {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = "."
	}
	d := core.Dirs{
		Data:    filepath.Join(base, "wisteria-desktop"),
		Runtime: filepath.Join(base, "wisteria-desktop", "runtime"),
	}
	// The 2026-10-23 rename (Wysteria -> Wisteria) also renamed the data
	// directory. Anyone who has already run the app keeps their profiles and
	// settings in the old one, so move it before creating the new one; without
	// this the first launch after an update looks like total data loss.
	migrateLegacyDir(filepath.Join(base, "wysteria-desktop"), d.Data)
	if err := os.MkdirAll(d.Runtime, 0o700); err != nil {
		return d, err
	}
	return d, nil
}

// migrateLegacyDir moves oldDir onto newDir, once. Best effort: if anything
// goes wrong we leave both directories alone and the app simply starts with an
// empty store, which is the same thing that would happen without this code.
func migrateLegacyDir(oldDir, newDir string) {
	if oldDir == newDir {
		return
	}
	if _, err := os.Stat(oldDir); err != nil {
		return // nothing to migrate
	}
	if _, err := os.Stat(newDir); err == nil {
		return // already migrated (or the user made the directory by hand)
	}
	_ = os.Rename(oldDir, newDir)
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	d, err := dataDirs()
	if err != nil {
		runtime.LogErrorf(ctx, "data dirs: %v", err)
	}
	a.dirs = d
	a.ring = core.NewLogRing(2000)
	fl := newFileLogger(d.Data)
	// The frontend pulls lines from the ring when it sees the "log" event,
	// so notify it at most ~3 times a second instead of once per core log
	// line: sing-box at DEBUG level under a system proxy produces hundreds
	// of lines per second, and each emit lands on the UI thread — enough to
	// wedge the message pump ("not responding" after a few hours).
	logWake := make(chan struct{}, 1)
	a.ring.SetHook(func(l core.LogLine) {
		if fl != nil {
			fl.write(l)
		}
		select {
		case logWake <- struct{}{}:
		default:
		}
	})
	go func() {
		for range logWake {
			runtime.EventsEmit(ctx, "log")
			time.Sleep(300 * time.Millisecond)
		}
	}()
	store, err := core.OpenStore(d.Data)
	if err != nil {
		// Into the ring (and app.log): the user must see that profiles were
		// set aside, not silently lost.
		a.logf("хранилище: %v", err)
	}
	a.store = store
	a.svc = core.NewService(d, a.ring, func() {
		st := a.svc.Status()
		runtime.EventsEmit(ctx, "state", st)
		// External IP check fires once per successful connect.
		if st.State == core.StateConnected && a.svc.Settings().CheckExtIP {
			go a.fetchAndEmitIP()
		}
	})
	a.svc.LoadSettings()
	core.SetGitHubToken(a.svc.Settings().GitHubToken)
	// First-run onboarding defaults: a VPN client is expected to take over
	// the system proxy (v2rayN/NekoBox behaviour). A stale proxy entry left
	// by another client otherwise keeps pointing browsers at a dead port.
	setPath := filepath.Join(d.Data, "settings.json")
	if _, err := os.Stat(setPath); os.IsNotExist(err) {
		_ = a.svc.SaveSettings(core.Settings{
			BridgeEnabled:  true,
			SysProxy:       true,
			CheckExtIP:     true,
			KillSwitch:     true, // ship-default; the zero value would persist an explicit off
			TunStrictRoute: true, // same for the strict-route ship-default
		})
		a.svc.LoadSettings()
	}
	a.logf("Wisteria Desktop %s запущено", appVersion)
	// A previous run may have died without teardown (crash, Task Manager
	// kill): the system proxy would still point at our dead HTTP port.
	// Only clear it when it actually points at our port.
	if core.ClearStaleSystemProxy(a.svc.Settings().HttpPort) {
		a.logf("сброшен протухший системный прокси от предыдущего запуска")
	}
	core.CleanupOldUpdate()
	a.startAutoPing()
	if !a.svc.Settings().NoUpdateCheck {
		go func() {
			if info, err := core.AppUpdateCheck(appVersion); err == nil && info.HasUpdate {
				a.logf("доступно обновление: v%s", info.Latest)
				runtime.EventsEmit(ctx, "appupdate", info)
			}
		}()
	}
	if a.svc.Settings().StartMinimized {
		runtime.WindowMinimise(ctx)
	}
	if set := a.svc.Settings(); set.AutoConnect != "" {
		if p, err := a.store.Get(set.AutoConnect); err == nil {
			a.logf("автоподключение: %s", p.Name)
			go func() { _ = a.svc.Start(p) }()
		}
	}
}

// fetchAndEmitIP resolves the exit address through the tunnel and pushes it
// to the UI (both success and failure land in the same event).
func (a *App) fetchAndEmitIP() {
	set := a.svc.Settings()
	ip, err := core.FetchExternalIP("127.0.0.1", set.LocalPort, 8*time.Second)
	if err != nil {
		a.logf("внешний IP: %v", err)
		runtime.EventsEmit(a.ctx, "extip", map[string]string{"error": err.Error()})
		return
	}
	a.logf("внешний IP: %s", ip)
	runtime.EventsEmit(a.ctx, "extip", map[string]string{"ip": ip})
}

// GetExternalIP lets the UI request a fresh check on demand.
func (a *App) GetExternalIP() (string, error) {
	st := a.svc.Status()
	if st.State != core.StateConnected {
		return "", errors.New("нет активного подключения")
	}
	return core.FetchExternalIP("127.0.0.1", a.svc.Settings().LocalPort, 8*time.Second)
}

// startAutoPing runs the background ping loop for the CONNECTED profile,
// measured through the tunnel. The other profiles are pinged manually only:
// a timed sweep over every endpoint would fire bare-TCP (or TLS) probes at
// foreign servers on a schedule — exactly the DPI-visible pattern the
// protocol avoids.
func (a *App) startAutoPing() {
	a.pingMu.Lock()
	defer a.pingMu.Unlock()
	if a.pingStop != nil {
		close(a.pingStop)
	}
	a.pingStop = nil
	sec := a.svc.Settings().AutoPingSec
	if sec <= 0 {
		return
	}
	stop := make(chan struct{})
	a.pingStop = stop
	go func() {
		t := time.NewTicker(time.Duration(sec) * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				st := a.svc.Status()
				if st.State != core.StateConnected {
					continue
				}
				p, err := a.store.Get(st.ProfileID)
				if err != nil {
					continue
				}
				ms, _ := core.PingEndpoint(core.PingTarget{
					Host:      p.Host,
					Port:      p.Port,
					Transport: p.Transport,
					TLSSNI:    p.TLSSNI,
					ViaSocks:  fmt.Sprintf("127.0.0.1:%d", a.svc.Settings().LocalPort),
				}, 3*time.Second)
				a.store.SetLatencies(map[string]int64{p.ID: ms})
				runtime.EventsEmit(a.ctx, "profiles")
			}
		}
	}()
}

// beforeClose asks for confirmation when a tunnel is still up (if enabled).
func (a *App) beforeClose(ctx context.Context) bool {
	if a.svc == nil {
		return false
	}
	set := a.svc.Settings()
	if set.ConfirmExit && a.svc.Status().State == core.StateConnected {
		quit, cancel := "Выйти", "Отмена"
		if set.Language == "en" {
			quit, cancel = "Quit", "Cancel"
		}
		res, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:          "question",
			Title:         "Wisteria Desktop",
			Message:       langText(set.Language, "Подключение активно. Выйти из приложения?", "A connection is active. Quit the app?"),
			Buttons:       []string{quit, cancel},
			DefaultButton: cancel,
			CancelButton:  cancel,
		})
		if err != nil {
			return true
		}
		return res != quit
	}
	return false
}

func langText(lang, ru, en string) string {
	if lang == "en" {
		return en
	}
	return ru
}

func (a *App) shutdown(ctx context.Context) {
	a.pingMu.Lock()
	if a.pingStop != nil {
		close(a.pingStop)
		a.pingStop = nil
	}
	a.pingMu.Unlock()
	if a.svc != nil {
		// Stop waits for the full teardown — the system proxy reset included
		// (it completes before the teardown-done signal).
		a.svc.Stop()
	}
}

func (a *App) logf(format string, args ...any) {
	if a.ring != nil {
		a.ring.Add("app", fmt.Sprintf(format, args...))
	}
}

// fileLogger appends every ring line to <data>/app.log. The in-memory ring
// dies with the process, so without this a crash or a stuck switch leaves
// nothing to diagnose afterwards.
type fileLogger struct {
	mu      sync.Mutex
	path    string
	f       *os.File
	written int64
}

func newFileLogger(dataDir string) *fileLogger {
	if dataDir == "" {
		return nil
	}
	path := filepath.Join(dataDir, "app.log")
	if st, err := os.Stat(path); err == nil && st.Size() > 2<<20 {
		_ = os.Remove(path) // rotate by truncation
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil
	}
	return &fileLogger{path: path, f: f}
}

// write appends one line, rotating in place once the file passes 8 MB:
// sing-box at debug level produces hundreds of lines per second, and the
// old size check ran only at startup — a long session could fill the disk.
func (fl *fileLogger) write(l core.LogLine) {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	if fl.f == nil {
		return
	}
	if fl.written >= 8<<20 {
		fl.f.Close()
		fl.f = nil
		fl.written = 0
		_ = os.Remove(fl.path)
		if f, err := os.OpenFile(fl.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			fl.f = f
		}
		if fl.f == nil {
			return
		}
	}
	n, _ := fmt.Fprintf(fl.f, "%s [%s] %s\n", l.At.Format("2006-01-02 15:04:05"), l.Tag, l.Text)
	fl.written += int64(n)
}

// ---- state / logs ----

func (a *App) GetState() core.Status {
	if a.svc == nil {
		return core.Status{State: core.StateDisconnected}
	}
	return a.svc.Status()
}

// GetWisteriaDiag feeds the diagnostics page: session state, the core's
// per-server probe rows and the classified connect-loop events.
func (a *App) GetWisteriaDiag() core.WisteriaDiag {
	if a.svc == nil {
		return core.WisteriaDiag{}
	}
	return a.svc.WisteriaDiag()
}

func (a *App) GetLogs(since int64) []core.LogLine {
	if a.ring == nil {
		return nil
	}
	return a.ring.Snapshot(since)
}

// ---- profiles ----

func (a *App) ListProfiles() []core.Profile {
	if a.store == nil {
		return nil
	}
	return a.store.List()
}

func (a *App) GetActiveID() string {
	if a.store == nil {
		return ""
	}
	if p, ok := a.store.Active(); ok {
		return p.ID
	}
	return ""
}

type LinkError struct {
	Line  string `json:"line"`
	Error string `json:"error"`
}

type AddResult struct {
	Added  []core.Profile `json:"added"`
	Errors []LinkError    `json:"errors"`
}

// AddLinks imports one or more share links (newline separated). A wisteria
// bundle link expands to one profile per server.
func (a *App) AddLinks(text string) AddResult {
	res := AddResult{Added: []core.Profile{}, Errors: []LinkError{}}
	for _, line := range splitLines(text) {
		profs, err := core.ParseLinkAll(line)
		if err != nil {
			res.Errors = append(res.Errors, LinkError{Line: shorten(line), Error: err.Error()})
			continue
		}
		for _, p := range profs {
			added, err := a.store.Add(p)
			if err != nil {
				res.Errors = append(res.Errors, LinkError{Line: shorten(line), Error: err.Error()})
				continue
			}
			res.Added = append(res.Added, added)
			a.logf("добавлен профиль %s [%s]", added.Name, added.Type)
		}
	}
	return res
}

// AddManual creates a profile from the structured form. Wisteria profiles
// get a fresh client key here (the server operator adds the public half to
// authorized_peers).
func (a *App) AddManual(p core.Profile) (core.Profile, error) {
	if !core.Supported(p.Type) {
		return core.Profile{}, fmt.Errorf("неподдерживаемый тип %q", p.Type)
	}
	if p.Host == "" || p.Port == 0 {
		return core.Profile{}, errors.New("нужны адрес и порт сервера")
	}
	if p.Name == "" {
		p.Name = p.Endpoint()
	}
	if p.Fields == nil {
		p.Fields = map[string]string{}
	}
	added, err := a.store.Add(p)
	if err != nil {
		return core.Profile{}, err
	}
	a.logf("добавлен профиль %s [%s]", added.Name, added.Type)
	return added, nil
}

// ImportTOML imports a wisteria client config pasted as text.
func (a *App) ImportTOML(text string) (core.Profile, error) {
	p, err := core.ParseWisteriaTOML(text)
	if err != nil {
		return core.Profile{}, err
	}
	added, err := a.store.Add(p)
	if err != nil {
		return core.Profile{}, err
	}
	a.logf("импортирован TOML-профиль %s", added.Name)
	return added, nil
}

func (a *App) RenameProfile(id, name string) error { return a.store.Rename(id, name) }

func (a *App) DeleteProfile(id string) error {
	if a.svc != nil && a.svc.RunningProfile() == id {
		return errors.New("профиль подключён — сначала отключитесь")
	}
	return a.store.Delete(id)
}

func (a *App) SetActive(id string) error { return a.store.SetActive(id) }

func (a *App) PingProfile(id string) (int64, error) {
	p, err := a.store.Get(id)
	if err != nil {
		return -1, err
	}
	// The connected profile is measured through the tunnel (the server sees
	// only tunnel traffic); the rest from here — TLS profiles get a real
	// handshake, legacy bare-TCP the old raw dial.
	tgt := core.PingTarget{Host: p.Host, Port: p.Port, Transport: p.Transport, TLSSNI: p.TLSSNI}
	if st := a.svc.Status(); st.State == core.StateConnected && st.ProfileID == id {
		tgt.ViaSocks = fmt.Sprintf("127.0.0.1:%d", a.svc.Settings().LocalPort)
	}
	ms, err := core.PingEndpoint(tgt, 3*time.Second)
	a.store.SetLatency(id, ms)
	return ms, err
}

// ---- subscriptions ----

func (a *App) ListSubscriptions() []core.Subscription {
	if a.store == nil {
		return nil
	}
	return a.store.Subscriptions()
}

func (a *App) AddSubscription(url, name string) (core.Subscription, error) {
	sub, err := a.store.AddSubscription(name, url)
	if err != nil {
		return sub, err
	}
	a.logf("добавлена подписка %s → %s", sub.Name, maskURL(sub.URL))
	return sub, nil
}

// RefreshSubscription downloads the list and merges it into the store.
// The returned subscription carries the fresh count or, on failure, the
// error text (the UI shows it inline); the fetch error is also returned.
func (a *App) RefreshSubscription(id string) (core.Subscription, error) {
	sub, err := a.store.GetSubscription(id)
	if err != nil {
		return sub, err
	}
	fetch, err := core.FetchSubscription(sub.URL, 15*time.Second)
	if err != nil {
		sub.Error = err.Error()
		_ = a.store.UpdateSubscription(sub)
		// Fetch errors embed the full request URL ("Get \"…?token=…\": …"),
		// and subscription URLs carry auth tokens — scrub before logging.
		a.logf("подписка %s: %s", sub.Name, scrubURLs(err.Error()))
		return sub, err
	}
	if _, err := a.store.ReplaceSubscriptionProfiles(id, fetch.Profiles); err != nil {
		return sub, err
	}
	sub.Count = len(fetch.Profiles)
	sub.UpdatedAt = time.Now()
	sub.Error = ""
	_ = a.store.UpdateSubscription(sub)
	a.logf("подписка %s обновлена: %d серверов (пропущено строк: %d)", sub.Name, sub.Count, fetch.Skipped)
	return sub, nil
}

func (a *App) DeleteSubscription(id string) error {
	if pid := a.svc.RunningProfile(); pid != "" {
		if p, err := a.store.Get(pid); err == nil && p.SourceID == id {
			return errors.New("подписка подключена — сначала отключитесь")
		}
	}
	err := a.store.DeleteSubscription(id)
	if err == nil {
		a.logf("подписка удалена вместе с её профилями")
	}
	return err
}

// CopyToClipboard puts text on the system clipboard. The Logs screen uses it
// so a user can send a full log instead of a stack of screenshots.
func (a *App) CopyToClipboard(text string) error {
	return runtime.ClipboardSetText(a.ctx, text)
}

// ClipImport is the summary of one paste-import from the clipboard.
type ClipImport struct {
	Subscriptions int            `json:"subscriptions"` // new subscriptions fetched
	Added         []core.Profile `json:"added"`         // profiles imported from links
	Errors        []LinkError    `json:"errors"`
}

// ImportFromClipboard reads the clipboard and routes every line: http(s)
// URLs become subscriptions (new ones are fetched at once), everything else
// is parsed as share links (wisteria bundles expand to one profile per
// server). One button for both kinds of content.
func (a *App) ImportFromClipboard() (ClipImport, error) {
	text, err := runtime.ClipboardGetText(a.ctx)
	if err != nil {
		return ClipImport{}, fmt.Errorf("буфер обмена недоступен: %v", err)
	}
	res := ClipImport{Added: []core.Profile{}, Errors: []LinkError{}}
	for _, line := range splitLines(text) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			sub, err := a.store.AddSubscription("", line)
			if err != nil {
				res.Errors = append(res.Errors, LinkError{Line: shorten(line), Error: err.Error()})
				continue
			}
			if sub.UpdatedAt.IsZero() && sub.Count == 0 {
				// Brand-new subscription: pull the servers right away so the
				// button is truly one step.
				if _, ferr := a.RefreshSubscription(sub.ID); ferr != nil {
					res.Errors = append(res.Errors, LinkError{Line: sub.Name, Error: ferr.Error()})
					continue
				}
			}
			res.Subscriptions++
			continue
		}
		profs, err := core.ParseLinkAll(line)
		if err != nil {
			res.Errors = append(res.Errors, LinkError{Line: shorten(line), Error: err.Error()})
			continue
		}
		for _, p := range profs {
			added, err := a.store.Add(p)
			if err != nil {
				res.Errors = append(res.Errors, LinkError{Line: shorten(line), Error: err.Error()})
				continue
			}
			res.Added = append(res.Added, added)
			a.logf("добавлен профиль %s [%s] (из буфера)", added.Name, added.Type)
		}
	}
	if res.Subscriptions == 0 && len(res.Added) == 0 && len(res.Errors) == 0 {
		return res, errors.New("в буфере нет ни ссылки-подписки, ни share-ссылок")
	}
	// The clipboard held credentials (share links carry client keys and
	// passwords, subscription URLs the auth tokens): once the import
	// consumed them, wipe the buffer. A failed import keeps it — the user
	// may want to fix the link and retry. Best effort: a failed wipe must
	// not fail the import.
	if res.Subscriptions > 0 || len(res.Added) > 0 {
		if err := runtime.ClipboardSetText(a.ctx, ""); err != nil {
			a.logf("буфер обмена не очищен после импорта: %v", err)
		}
	}
	return res, nil
}

// ---- connection ----

// Connect moves the tunnel to the profile. The active marker follows the
// connection, not the click: if the new session fails to start, the check
// in the profiles list stays on the profile that is actually running.
func (a *App) Connect(id string) error {
	p, err := a.store.Get(id)
	if err != nil {
		return err
	}
	prev := ""
	if ap, ok := a.store.Active(); ok {
		prev = ap.ID
	}
	if err := a.svc.Start(p); err != nil {
		if prev != "" && prev != id {
			_ = a.store.SetActive(prev)
		}
		return err
	}
	return a.store.SetActive(id)
}

func (a *App) Disconnect() { a.svc.Stop() }

// ToggleSystemProxy flips the Windows proxy while connected (the bridge
// must be up; the address is the current session's).
func (a *App) ToggleSystemProxy(on bool) error {
	if !core.SystemProxySupported() {
		return errors.New("системный прокси на этой системе не поддерживается")
	}
	st := a.svc.Status()
	if on && st.State != core.StateConnected {
		return errors.New("сначала подключитесь")
	}
	addr := st.HttpAddr
	if !on {
		addr = ""
	}
	if err := core.SetSystemProxy(on, addr); err != nil {
		return err
	}
	a.svc.SetSystemProxyState(on)
	a.logf("системный прокси вручную: %v", on)
	runtime.EventsEmit(a.ctx, "state", a.svc.Status())
	return nil
}

// ---- settings ----

func (a *App) GetSettings() core.Settings { return a.svc.Settings() }

func (a *App) SaveSettings(s core.Settings) core.Settings {
	out := a.svc.SaveSettings(s)
	core.SetGitHubToken(out.GitHubToken)
	a.startAutoPing() // interval may have changed
	a.logf("настройки сохранены")
	return out
}

type CoreInfo struct {
	WisteriaFound bool   `json:"wisteria_found"`
	WisteriaPath  string `json:"wisteria_path"`
	SingBoxFound  bool   `json:"singbox_found"`
	SingBoxPath   string `json:"singbox_path"`
	XrayFound     bool   `json:"xray_found"`
	XrayPath      string `json:"xray_path"`
}

// DetectCores reports which core binaries are resolvable right now.
func (a *App) DetectCores() CoreInfo {
	set := a.svc.Settings()
	info := CoreInfo{}
	if p := firstDefined(set.WisteriaCore, core.FindCore("wisteria-client")); p != "" {
		info.WisteriaFound, info.WisteriaPath = true, p
	}
	if p := firstDefined(set.SingBoxCore, core.FindCore("sing-box")); p != "" {
		info.SingBoxFound, info.SingBoxPath = true, p
	}
	if p := firstDefined(set.XrayCore, core.FindCore("xray")); p != "" {
		info.XrayFound, info.XrayPath = true, p
	}
	return info
}

// IsElevated reports whether the app runs with administrator rights
// (required for the TUN mode).
func (a *App) IsElevated() bool { return core.IsElevated() }

func (a *App) GetAutostart() bool { return core.AutostartEnabled() }

func (a *App) SetAutostart(on bool) error {
	err := core.SetAutostart(on)
	if err == nil {
		a.logf("автозапуск: %v", on)
	}
	return err
}

func (a *App) Version() string { return appVersion }

// ---- updates ----

func (a *App) CheckAppUpdate() (core.UpdateInfo, error) {
	return core.AppUpdateCheck(appVersion)
}

// ApplyAppUpdate stages the new release (verified, checksummed) and returns
// the version it installed; the UI then offers a restart.
func (a *App) ApplyAppUpdate() (string, error) {
	newVer, err := core.AppUpdateApply(appVersion)
	if err == nil {
		a.logf("приложение обновлено до v%s — готово к перезапуску", newVer)
	}
	return newVer, err
}

// RestartApp relaunches the (already staged) binary and quits this instance.
func (a *App) RestartApp() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe)
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		return err
	}
	a.logf("перезапуск после обновления...")
	go func() {
		time.Sleep(300 * time.Millisecond)
		runtime.Quit(a.ctx)
	}()
	return nil
}

// coreExplicitPath returns the user-configured binary path for a core
// (empty = the app's own bin/ copy).
func (a *App) coreExplicitPath(name string) string {
	switch name {
	case "sing-box":
		return a.svc.Settings().SingBoxCore
	case "xray":
		return a.svc.Settings().XrayCore
	case "wisteria":
		return a.svc.Settings().WisteriaCore
	}
	return ""
}

// CheckCoreUpdate reports the latest upstream version of
// "sing-box"/"xray"/"wisteria".
func (a *App) CheckCoreUpdate(name string) (core.UpdateInfo, error) {
	return core.CoreUpdateCheck(name, a.coreExplicitPath(name))
}

// UpdateCore replaces the app's own bin/<name>.exe with the latest upstream
// release. Refuses while a session is up (the binary may be in use) and for
// externally configured core paths.
func (a *App) UpdateCore(name string) (string, error) {
	if a.svc.RunningProfile() != "" {
		return "", errors.New("подключение активно — сначала отключитесь")
	}
	ver, err := core.CoreUpdateApply(name, a.coreExplicitPath(name))
	if err == nil {
		a.logf("ядро %s обновлено до v%s", name, ver)
	}
	return ver, err
}

// ---- helpers ----

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' || s[i] == '\r' {
			if seg := trimSpace(s[start:i]); seg != "" {
				out = append(out, seg)
			}
			start = i + 1
		}
	}
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func shorten(s string) string {
	r := []rune(s)
	if len(r) > 42 {
		return string(r[:42]) + "…"
	}
	return s
}

// maskURL drops the query and fragment: subscription URLs commonly carry
// an auth token there, and the raw URL otherwise lands in the log ring and
// app.log.
func maskURL(u string) string {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		return u[:i]
	}
	return u
}

// urlInText matches an http(s) URL inside free text (fetch errors quote the
// request URL; it ends at the closing quote or whitespace).
var urlInText = regexp.MustCompile(`\bhttps?://[^\s"'<>]+`)

// scrubURLs strips the query and fragment from every http(s) URL in s —
// the same masking maskURL applies, for error strings that embed whole
// URLs (redirects included, not just the subscription's own).
func scrubURLs(s string) string {
	return urlInText.ReplaceAllStringFunc(s, maskURL)
}

func firstDefined(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
