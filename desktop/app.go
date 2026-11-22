package main

// App: the API surface bound to the frontend (window.go.main.App.*).
// Thin on purpose — the logic lives in desktop/internal/core.

import (
	"bufio"
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

const appVersion = "1.1.24"

type App struct {
	ctx      context.Context
	svc      *core.Service
	store    *core.Store
	ring     *core.LogRing
	fl       *fileLogger
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
	a.ring = core.NewLogRing(4000)
	hydrateRingFromFile(a.ring, filepath.Join(d.Data, "app.log"))
	fl := newFileLogger(d.Data)
	a.fl = fl
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
	})
	a.svc.LoadSettings()
	core.SetGitHubToken(a.svc.Settings().GitHubToken)
	// First-run onboarding defaults: a VPN client is expected to take over
	// the system proxy (v2rayN/NekoBox behaviour). A stale proxy entry left
	// by another client otherwise keeps pointing browsers at a dead port.
	setPath := filepath.Join(d.Data, "settings.json")
	if _, err := os.Stat(setPath); os.IsNotExist(err) {
		_ = a.svc.SaveSettings(core.Settings{
			BridgeEnabled: true,
			SysProxy:      true,
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

// GetExternalIP lets the UI request a fresh check on demand.
func (a *App) GetExternalIP() (string, error) {
	st := a.svc.Status()
	if st.State != core.StateConnected {
		return "", errors.New("нет активного подключения")
	}
	return core.FetchExternalIP("127.0.0.1", a.svc.Settings().LocalPort, 8*time.Second)
}

// startAutoPing runs the background TCP-ping loop over all profiles.
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
			}
			// One batched store write per cycle: per-profile SetLatency
			// would rewrite profiles.json N times per tick.
			profiles := a.store.List()
			lat := make(map[string]int64, len(profiles))
			var mu sync.Mutex
			var wg sync.WaitGroup
			// Bounded parallelism: sequential pings let a handful of dead
			// servers stall the sweep for 3s each (20 offline profiles = a
			// minute of nothing). The stop check before acquiring a slot
			// keeps shutdown instant even with every slot busy.
			sem := make(chan struct{}, 8)
			for _, p := range profiles {
				select {
				case <-stop:
					return
				case sem <- struct{}{}:
				}
				wg.Add(1)
				go func(id, host string, port int) {
					defer wg.Done()
					defer func() { <-sem }()
					ms, _ := core.PingEndpoint(host, port, 3*time.Second)
					mu.Lock()
					lat[id] = ms
					mu.Unlock()
				}(p.ID, p.Host, p.Port)
			}
			wg.Wait()
			select {
			case <-stop:
				return
			default:
			}
			a.store.SetLatencies(lat)
			runtime.EventsEmit(a.ctx, "profiles")
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

func (fl *fileLogger) truncate() {
	if fl == nil {
		return
	}
	fl.mu.Lock()
	defer fl.mu.Unlock()
	if fl.f != nil {
		_ = fl.f.Close()
		fl.f = nil
	}
	fl.written = 0
	_ = os.Remove(fl.path)
	if f, err := os.OpenFile(fl.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		fl.f = f
	}
}

// hydrateRingFromFile loads the last 24 hours of logs from app.log into the
// in-memory ring and rewrites app.log without expired entries.
func hydrateRingFromFile(ring *core.LogRing, path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-core.LogRetention)
	var kept []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 512*1024)
	for sc.Scan() {
		line := sc.Text()
		if len(line) < 22 {
			continue
		}
		at, err := time.ParseInLocation("2006-01-02 15:04:05", line[:19], time.Local)
		if err != nil || at.Before(cutoff) {
			continue
		}
		rest := line[20:]
		tag := "app"
		text := rest
		if strings.HasPrefix(rest, "[") {
			if end := strings.Index(rest, "] "); end > 1 {
				tag = rest[1:end]
				text = rest[end+2:]
			}
		}
		ring.AddAt(tag, text, at)
		kept = append(kept, line)
	}
	_ = f.Close()
	if len(kept) == 0 {
		_ = os.Remove(path)
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, []byte(strings.Join(kept, "\n")+"\n"), 0o600) == nil {
		_ = os.Rename(tmp, path)
	}
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

// ClearLogs clears the in-memory ring, app.log and 24h diagnostics feed.
func (a *App) ClearLogs() {
	if a.ring != nil {
		a.ring.Clear()
	}
	if a.fl != nil {
		a.fl.truncate()
	}
	if a.svc != nil {
		a.svc.ClearDiag()
	}
}

// GetCoreHealth feeds the live health line above the logs: the latest
// rtt/loss/pings slice the wisteria core reported (consumed from the log
// stream, not logged). Zero value when no session has reported yet.
func (a *App) GetCoreHealth() core.CoreHealth {
	if a.svc == nil {
		return core.CoreHealth{}
	}
	return a.svc.CoreHealth()
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

// SetProfilePreset stores the per-profile obfuscation preset ("" clears it so
// the global default applies again). A live session is rebuilt right away —
// the preset is baked into the core config at connect time.
func (a *App) SetProfilePreset(id, preset string) error {
	if preset != "" && !core.ValidPreset(preset) {
		return fmt.Errorf("unknown preset %q (fast|balance|stealth)", preset)
	}
	if err := a.store.SetField(id, "preset", preset); err != nil {
		return err
	}
	if a.svc.RunningProfile() == id {
		a.reconnectRunning(id)
	}
	return nil
}

// reconnectRunning rebuilds the live session of id in the background: Start
// replaces a running session safely, and a reconnect failure must not reject
// the UI call that merely changed a setting.
func (a *App) reconnectRunning(id string) {
	go func() {
		a.logf("настройки изменены — переподключение")
		if err := a.Connect(id); err != nil {
			a.logf("автопереподключение после смены настроек: %v", err)
		}
	}()
}

// effPreset mirrors the core rule: a per-profile preset wins over the global.
func (a *App) effPreset(s core.Settings, id string) string {
	v := s.WisteriaPreset
	if p, err := a.store.Get(id); err == nil && p.Fields["preset"] != "" {
		v = p.Fields["preset"]
	}
	if !core.ValidPreset(v) {
		v = "stealth"
	}
	return v
}

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
	ms, err := core.PingEndpoint(p.Host, p.Port, 3*time.Second)
	a.store.SetLatency(id, ms)
	return ms, err
}

// EditProfilePayload carries the editable fields from the Edit Profile modal.
type EditProfilePayload struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Transport   string `json:"transport"`
	TLSSNI      string `json:"tls_sni"`
	TLSInsecure bool   `json:"tls_insecure"`
}

func (a *App) EditProfile(req EditProfilePayload) (core.Profile, error) {
	p, err := a.store.UpdateProfile(req.ID, req.Name, req.Host, req.Port, req.Transport, req.TLSSNI, req.TLSInsecure)
	if err != nil {
		return core.Profile{}, err
	}
	if a.svc != nil && a.svc.RunningProfile() == req.ID {
		a.reconnectRunning(req.ID)
	}
	return p, nil
}

// PingAllProfiles TCP-pings every stored profile concurrently (bounded to 8
// workers) and writes the batch to profiles.json in a single save.
func (a *App) PingAllProfiles() map[string]int64 {
	profiles := a.store.List()
	lat := make(map[string]int64, len(profiles))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, p := range profiles {
		sem <- struct{}{}
		wg.Add(1)
		go func(id, host string, port int) {
			defer wg.Done()
			defer func() { <-sem }()
			ms, _ := core.PingEndpoint(host, port, 3*time.Second)
			mu.Lock()
			lat[id] = ms
			mu.Unlock()
		}(p.ID, p.Host, p.Port)
	}
	wg.Wait()
	a.store.SetLatencies(lat)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "profiles")
	}
	return lat
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
	if fetch.Userinfo != nil {
		sub.Userinfo = fetch.Userinfo
	}
	if fetch.Title != "" {
		sub.Name = fetch.Title
	}
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
	// WinINET cannot use 0.0.0.0 as a proxy host: with ListenAll the
	// displayed HttpAddr is the bind address, but the proxy runs locally.
	addr := ""
	if on {
		addr = fmt.Sprintf("127.0.0.1:%d", a.svc.Settings().HttpPort)
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
	old := a.svc.Settings()
	out := a.svc.SaveSettings(s)
	core.SetGitHubToken(out.GitHubToken)
	a.startAutoPing() // interval may have changed
	a.logf("настройки сохранены")
	// Anything the running core/bridge/proxy/TUN helper snapshotted at
	// connect time needs a reconnect to take effect. Profiles can carry
	// their own preset and engine, so resolve those against the live profile
	// rather than only the global settings.
	if id := a.svc.RunningProfile(); id != "" {
		prof, err := a.store.Get(id)
		engineChanged := err == nil && old.EngineFor(prof.Type) != out.EngineFor(prof.Type)
		changed := old.LocalPort != out.LocalPort ||
			old.HttpPort != out.HttpPort ||
			old.ListenAll != out.ListenAll ||
			old.BridgeEnabled != out.BridgeEnabled ||
			old.SysProxy != out.SysProxy ||
			old.BypassLAN != out.BypassLAN ||
			old.NoBypassRU != out.NoBypassRU ||
			old.RemoteDNS != out.RemoteDNS ||
			old.LogLevel != out.LogLevel ||
			old.MemLimitMB != out.MemLimitMB ||
			old.RouteDirect != out.RouteDirect ||
			old.RouteBlock != out.RouteBlock ||
			old.TunEnabled != out.TunEnabled ||
			old.TunStack != out.TunStack ||
			old.TunStrictRoute != out.TunStrictRoute ||
			engineChanged ||
			a.effPreset(old, id) != a.effPreset(out, id)
		if changed {
			a.reconnectRunning(id)
		}
	}
	return out
}

type CoreInfo struct {
	OS            string `json:"os"` // "windows" | "linux" (wintun row is Windows-only)
	WisteriaFound bool   `json:"wisteria_found"`
	WisteriaPath  string `json:"wisteria_path"`
	SingBoxFound  bool   `json:"singbox_found"`
	SingBoxPath   string `json:"singbox_path"`
	XrayFound     bool   `json:"xray_found"`
	XrayPath      string `json:"xray_path"`
	// WintunFound/Path: the DLL sing-box's TUN helper needs on Windows.
	WintunFound bool   `json:"wintun_found"`
	WintunPath  string `json:"wintun_path"`
}

// DetectCores reports which core binaries are resolvable right now.
func (a *App) DetectCores() CoreInfo {
	set := a.svc.Settings()
	info := CoreInfo{OS: core.RuntimeOS()}
	if p := firstDefined(set.WisteriaCore, core.FindCore("wisteria-client")); p != "" {
		info.WisteriaFound, info.WisteriaPath = true, p
	}
	if p := firstDefined(set.SingBoxCore, core.FindCore("sing-box")); p != "" {
		info.SingBoxFound, info.SingBoxPath = true, p
	}
	if p := firstDefined(set.XrayCore, core.FindCore("xray")); p != "" {
		info.XrayFound, info.XrayPath = true, p
	}
	// The same resolved sing-box and lookup order the TUN gate uses, so
	// the note mirrors what a connect would actually find.
	if info.OS == "windows" {
		if p := core.WintunPath(info.SingBoxPath, a.dirs); p != "" {
			info.WintunFound, info.WintunPath = true, p
		}
	}
	return info
}

// TunStatus is the VPN tab's TUN readiness snapshot: what the helper needs
// and what is missing right now. The UI renders one actionable row per
// missing prerequisite instead of letting Connect fail with a generic error.
type TunStatus struct {
	OS           string `json:"os"`
	Elevated     bool   `json:"elevated"` // admin on Windows, root on Linux
	HasCap       bool   `json:"has_cap"`  // Linux: root or cap_net_admin on sing-box
	SingBoxFound bool   `json:"singbox_found"`
	SingBoxPath  string `json:"singbox_path,omitempty"`
	WintunFound  bool   `json:"wintun_found"`
	Ready        bool   `json:"ready"`
}

// TunPreflight reports whether a TUN connect can succeed right now. It
// mirrors the startTun gate (same core resolution, same wintun lookup, same
// capability check) so the card can never promise a setup that Connect
// would still reject.
func (a *App) TunPreflight() TunStatus {
	info := a.DetectCores()
	st := TunStatus{
		OS:           info.OS,
		Elevated:     core.IsElevated(),
		SingBoxFound: info.SingBoxFound,
		SingBoxPath:  info.SingBoxPath,
	}
	switch info.OS {
	case "windows":
		st.HasCap = st.Elevated
		st.WintunFound = info.WintunFound
		st.Ready = st.Elevated && st.SingBoxFound && st.WintunFound
	case "linux":
		st.HasCap = core.TunAvailable(info.SingBoxPath)
		st.Ready = st.HasCap && st.SingBoxFound
	default:
		st.HasCap = st.Elevated
		st.Ready = st.HasCap && st.SingBoxFound
	}
	return st
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
// On an AppImage the staged file is the image, not the path the process
// reports: os.Executable() there is the old squashfs mount, which would
// relaunch the previous version (and vanish with this process).
func (a *App) RestartApp() error {
	target := core.AppImagePath()
	if target == "" {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		target = exe
	}
	cmd := exec.Command(target)
	cmd.Dir = filepath.Dir(target)
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
	if err != nil {
		return ver, err
	}
	// One "download sing-box" click must leave TUN usable: upstream
	// stopped bundling wintun.dll, so when the DLL gate would fail after
	// the fresh install, the pinned official copy goes in right away.
	// A failure here is logged, not returned — sing-box itself installed
	// fine, and the settings screen offers a retry button.
	if name == "sing-box" && core.RuntimeOS() == "windows" {
		sb := firstDefined(a.svc.Settings().SingBoxCore, core.FindCore("sing-box"))
		if !core.WintunInstalled(sb, a.dirs) {
			if _, werr := core.WintunApply(sb, a.dirs); werr != nil {
				a.logf("wintun.dll: %v — скачайте вручную в Настройках → Ядра", werr)
			} else {
				a.logf("wintun.dll установлен для TUN")
			}
		}
	}
	a.logf("ядро %s обновлено до v%s", name, ver)
	return ver, nil
}

// WintunInstall downloads the pinned official wintun.dll into the first
// directory sing-box reads it from (Windows only — the settings screen
// hides the button elsewhere). Refuses while a session runs.
func (a *App) WintunInstall() (string, error) {
	if a.svc.RunningProfile() != "" {
		return "", errors.New("подключение активно — сначала отключитесь")
	}
	// The same resolved sing-box path DetectCores and UpdateCore use: an
	// empty setting must not send the DLL to a directory the TUN gate will
	// never scan.
	sb := firstDefined(a.svc.Settings().SingBoxCore, core.FindCore("sing-box"))
	dir, err := core.WintunApply(sb, a.dirs)
	if err == nil {
		a.logf("wintun.dll установлен в %s", dir)
	}
	return dir, err
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
