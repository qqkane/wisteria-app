package core

// Profile store: a single JSON file in the app-data directory, written with
// 0600 (best-effort on Windows). The active profile id is persisted too, so
// a restart can offer auto-connect.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("profile not found")

// Subscription is a remote list of share links (the v2rayN "subscription"
// convention): a URL that is re-fetched on demand and merged into the
// profile list. Profiles imported from a subscription carry SourceID, so a
// delete of the subscription removes exactly its own profiles.
type Subscription struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Count     int       `json:"count"` // profiles of the last successful refresh
	UpdatedAt time.Time `json:"updated_at"`
	Error     string    `json:"error,omitempty"` // last refresh error ("" = ok)
}

type Store struct {
	mu       sync.Mutex
	path     string
	Profiles []Profile      `json:"profiles"`
	Subs     []Subscription `json:"subs,omitempty"`
	ActiveID string         `json:"active"`
}

func OpenStore(appDataDir string) (*Store, error) {
	s := &Store{path: filepath.Join(appDataDir, "profiles.json")}
	data, err := os.ReadFile(s.path)
	if err == nil {
		var disk Store
		if jerr := json.Unmarshal(data, &disk); jerr == nil {
			s.Profiles = disk.Profiles
			s.Subs = disk.Subs // without this subscriptions vanish on every restart
			s.ActiveID = disk.ActiveID
			normalizeLegacyTypes(s.Profiles)
		} else {
			// An unparseable store must survive the next save: saveLocked
			// renames over profiles.json, which would replace the corrupt
			// file (and every profile in it) with an empty one. Rename it
			// aside and report; the app then starts empty but recoverable.
			backup := s.path + ".corrupt"
			_ = os.Remove(backup)
			if rerr := os.Rename(s.path, backup); rerr == nil {
				return s, fmt.Errorf("profiles.json повреждён (%v) — копия сохранена: %s", jerr, backup)
			}
			return s, fmt.Errorf("profiles.json повреждён (%v) и копию сохранить не удалось", jerr)
		}
	}
	return s, nil
}

// normalizeLegacyTypes maps profile type ids saved before the 2026-10-23
// rename ("wysteria") onto the current one. Without this, every wisteria
// profile stored by an older build stops being Supported after the update.
func normalizeLegacyTypes(ps []Profile) {
	for i := range ps {
		if ps[i].Type == "wysteria" {
			ps[i].Type = TWisteria
		}
	}
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	// Write to a sibling temp file and rename over the store: a plain
	// WriteFile truncated profiles.json in place, so a crash or power loss
	// mid-write (the auto-ping tick rewrites the file every few seconds)
	// destroyed every profile at once. Go's rename over an existing file
	// replaces it atomically on both Windows and Unix.
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (s *Store) List() []Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Profile, len(s.Profiles))
	copy(out, s.Profiles)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (s *Store) Active() (Profile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.Profiles {
		if p.ID == s.ActiveID {
			return p, true
		}
	}
	return Profile{}, false
}

// Add stores the profile, assigning an id (and a fresh wisteria client key
// when the profile does not carry one). Re-adding the same raw link is a
// no-op returning the existing profile. Every import path funnels through
// here, so the TLS fields are validated+canonicalized at the single choke
// point (a manual-form or hand-edited-store profile skips the parsers).
func (s *Store) Add(p Profile) (Profile, error) {
	if err := p.normalizeTLS(); err != nil {
		return Profile{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.Raw != "" {
		for _, ex := range s.Profiles {
			if ex.Raw == p.Raw && ex.Type == p.Type {
				return ex, nil
			}
		}
	}
	if p.ID == "" {
		p.ID = newID()
	}
	if p.Type == TWisteria && p.ClientPriv == "" {
		priv, _, err := GenerateX25519()
		if err != nil {
			return Profile{}, err
		}
		p.ClientPriv = priv
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	s.Profiles = append(s.Profiles, p)
	if s.ActiveID == "" {
		s.ActiveID = p.ID
	}
	return p, s.saveLocked()
}

func (s *Store) Rename(id, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Profiles {
		if s.Profiles[i].ID == id {
			s.Profiles[i].Name = name
			return s.saveLocked()
		}
	}
	return ErrNotFound
}

// Delete removes the profile. Removing the active profile clears the
// pointer; the caller is responsible for not deleting a connected profile
// (the service layer refuses).
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Profiles {
		if s.Profiles[i].ID == id {
			s.Profiles = append(s.Profiles[:i], s.Profiles[i+1:]...)
			if s.ActiveID == id {
				s.ActiveID = ""
			}
			return s.saveLocked()
		}
	}
	return ErrNotFound
}

func (s *Store) Get(id string) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.Profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return Profile{}, ErrNotFound
}

func (s *Store) SetActive(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	for _, p := range s.Profiles {
		if p.ID == id {
			found = true
			break
		}
	}
	if !found {
		return ErrNotFound
	}
	s.ActiveID = id
	return s.saveLocked()
}

// SetLatency records the last TCP ping result (-1 = failure).
func (s *Store) SetLatency(id string, ms int64) {
	s.SetLatencies(map[string]int64{id: ms})
}

// SetLatencies records a batch of ping results with a single file write —
// the auto-ping loop updates every profile on each tick.
func (s *Store) SetLatencies(ms map[string]int64) {
	if len(ms) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Profiles {
		if v, ok := ms[s.Profiles[i].ID]; ok {
			s.Profiles[i].LatencyMs = v
		}
	}
	_ = s.saveLocked()
}

// ---- subscriptions ----

func (s *Store) AddSubscription(name, rawURL string) (Subscription, error) {
	rawURL = strings.TrimSpace(rawURL)
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Subscription{}, errors.New("подписка должна быть http(s)-URL")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ex := range s.Subs {
		if ex.URL == rawURL {
			return ex, nil
		}
	}
	if name == "" {
		name = u.Host
	}
	sub := Subscription{ID: newID(), Name: name, URL: rawURL}
	s.Subs = append(s.Subs, sub)
	return sub, s.saveLocked()
}

func (s *Store) Subscriptions() []Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Subscription, len(s.Subs))
	copy(out, s.Subs)
	return out
}

func (s *Store) GetSubscription(id string) (Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.Subs {
		if x.ID == id {
			return x, nil
		}
	}
	return Subscription{}, ErrNotFound
}

// UpdateSubscription persists refreshed metadata (count/time/error) keyed
// by the subscription id.
func (s *Store) UpdateSubscription(sub Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Subs {
		if s.Subs[i].ID == sub.ID {
			sub.URL = s.Subs[i].URL // the URL is edited by delete+re-add only
			s.Subs[i] = sub
			return s.saveLocked()
		}
	}
	return ErrNotFound
}

// DeleteSubscription removes the subscription and every profile imported
// from it. Removing the active profile clears the pointer; the caller is
// responsible for not deleting a connected subscription.
func (s *Store) DeleteSubscription(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	subs := s.Subs[:0]
	for _, x := range s.Subs {
		if x.ID == id {
			found = true
			continue
		}
		subs = append(subs, x)
	}
	if !found {
		return ErrNotFound
	}
	s.Subs = subs
	out := s.Profiles[:0]
	for _, p := range s.Profiles {
		if p.SourceID == id {
			continue
		}
		out = append(out, p)
	}
	s.Profiles = out
	if s.ActiveID != "" {
		still := false
		for _, p := range s.Profiles {
			if p.ID == s.ActiveID {
				still = true
				break
			}
		}
		if !still {
			s.ActiveID = ""
		}
	}
	return s.saveLocked()
}

// subKey identifies a server across refreshes: same protocol and endpoint
// is the same node with updated credentials/params, so its store identity
// (id, latency, wisteria client key) survives.
func subKey(p *Profile) string { return string(p.Type) + "|" + p.Endpoint() }

// ReplaceSubscriptionProfiles merges a freshly fetched list into the store
// as the current content of the subscription. Old profiles of this sub that
// are gone from the list are removed; ones still present keep their id,
// latency and wisteria client key (the key must survive: the server knows
// the client by its public half). Profiles from other sources are untouched.
// Returns the number of brand-new profiles.
func (s *Store) ReplaceSubscriptionProfiles(subID string, fresh []Profile) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reusable := map[string]*Profile{}
	out := make([]Profile, 0, len(s.Profiles)+len(fresh))
	for i := range s.Profiles {
		if s.Profiles[i].SourceID == subID {
			reusable[subKey(&s.Profiles[i])] = &s.Profiles[i]
		} else {
			out = append(out, s.Profiles[i])
		}
	}
	now := time.Now()
	added := 0
	for _, np := range fresh {
		np.SourceID = subID
		if err := np.normalizeTLS(); err != nil {
			return 0, err
		}
		if np.Fields == nil {
			np.Fields = map[string]string{}
		}
		if ex := reusable[subKey(&np)]; ex != nil {
			delete(reusable, subKey(&np))
			np.ID, np.CreatedAt, np.LatencyMs = ex.ID, ex.CreatedAt, ex.LatencyMs
			if np.Type == TWisteria {
				np.ClientPriv = ex.ClientPriv
				if np.ClientPriv == "" {
					priv, _, err := GenerateX25519()
					if err != nil {
						return 0, err
					}
					np.ClientPriv = priv
				}
			}
			out = append(out, np)
			continue
		}
		if np.ID == "" {
			np.ID = newID()
		}
		if np.Type == TWisteria && np.ClientPriv == "" {
			priv, _, err := GenerateX25519()
			if err != nil {
				return 0, err
			}
			np.ClientPriv = priv
		}
		if np.CreatedAt.IsZero() {
			np.CreatedAt = now
		}
		out = append(out, np)
		added++
	}
	s.Profiles = out
	if s.ActiveID != "" {
		still := false
		for _, p := range s.Profiles {
			if p.ID == s.ActiveID {
				still = true
				break
			}
		}
		if !still {
			s.ActiveID = ""
		}
	}
	return added, s.saveLocked()
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("150405.000000000")))
	}
	return hex.EncodeToString(b[:])
}
