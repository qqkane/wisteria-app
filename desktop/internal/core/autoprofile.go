package core

import (
	"errors"
	"sort"
	"sync"
	"time"
)

// AutoProfileID is the virtual profile ID representing automatic failover
// across all configured profiles.
const AutoProfileID = "__auto__"

// SetProfileProvider registers a callback returning the current list of profiles
// so Service can fail over across servers when AutoProfile mode is active.
func (s *Service) SetProfileProvider(fn func() []Profile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profileProvider = fn
}

// rankAutoCandidates orders profiles for auto-failover:
// 1. Positive ping (lowest ms first)
// 2. Untested ping (0 ms, preserving list order)
// 3. Failed ping (< 0 ms, preserving list order)
func rankAutoCandidates(profiles []Profile) []Profile {
	type indexed struct {
		p   Profile
		idx int
	}
	items := make([]indexed, 0, len(profiles))
	for i, p := range profiles {
		if p.ID == "" || p.ID == AutoProfileID {
			continue
		}
		items = append(items, indexed{p: p, idx: i})
	}
	tier := func(ms int64) int {
		if ms > 0 {
			return 0
		}
		if ms == 0 {
			return 1
		}
		return 2
	}
	sort.SliceStable(items, func(i, j int) bool {
		ti, tj := tier(items[i].p.LatencyMs), tier(items[j].p.LatencyMs)
		if ti != tj {
			return ti < tj
		}
		if ti == 0 && items[i].p.LatencyMs != items[j].p.LatencyMs {
			return items[i].p.LatencyMs < items[j].p.LatencyMs
		}
		return items[i].idx < items[j].idx
	})
	out := make([]Profile, len(items))
	for i, it := range items {
		out[i] = it.p
	}
	return out
}

// StartAuto enables Auto-Profile failover mode and connects to the best
// available profile. If the active server drops or fails to connect, the
// reconnect loop automatically switches to the next working profile.
func (s *Service) StartAuto() error {
	s.mu.Lock()
	fn := s.profileProvider
	s.mu.Unlock()
	if fn == nil {
		return errors.New("автопрофиль: список профилей недоступен")
	}
	profiles := fn()
	if len(profiles) == 0 {
		return errors.New("нет профилей для автоподключения — сначала добавьте сервер")
	}
	// If no profile has a measured latency yet and there are multiple candidates,
	// run a fast concurrent TCP probe so the first connect picks a live node.
	hasMeasured := false
	for _, p := range profiles {
		if p.LatencyMs != 0 {
			hasMeasured = true
			break
		}
	}
	if !hasMeasured && len(profiles) > 1 {
		var wg sync.WaitGroup
		var mu sync.Mutex
		sem := make(chan struct{}, 8)
		for i := range profiles {
			sem <- struct{}{}
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				defer func() { <-sem }()
				ms, _ := PingEndpoint(profiles[idx].Host, profiles[idx].Port, 1200*time.Millisecond)
				mu.Lock()
				profiles[idx].LatencyMs = ms
				mu.Unlock()
			}(i)
		}
		wg.Wait()
	}

	ranked := rankAutoCandidates(profiles)
	if len(ranked) == 0 {
		return errors.New("нет доступных профилей для автоподключения")
	}
	chosen := ranked[0]

	s.mu.Lock()
	s.reconnGen++
	s.reconnNext = 0
	s.autoMode = true
	s.autoTried = map[string]bool{chosen.ID: true}
	s.st.AutoProfile = true
	s.mu.Unlock()

	s.logf("автопрофиль: выбран «%s» (%s)", chosen.Name, chosen.Endpoint())
	return s.start(chosen, false, 0)
}

func (s *Service) pickNextAutoProfile(failedID string) (Profile, bool) {
	s.mu.Lock()
	fn := s.profileProvider
	auto := s.autoMode
	s.mu.Unlock()
	if !auto || fn == nil {
		return Profile{}, false
	}
	ranked := rankAutoCandidates(fn())
	if len(ranked) == 0 {
		return Profile{}, false
	}
	if len(ranked) == 1 {
		return ranked[0], true
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.autoTried == nil {
		s.autoTried = make(map[string]bool)
	}
	if failedID != "" {
		s.autoTried[failedID] = true
	}
	for _, cand := range ranked {
		if !s.autoTried[cand.ID] {
			s.autoTried[cand.ID] = true
			return cand, true
		}
	}
	// All candidates were tried once — start a fresh rotation round, still
	// preferring any server other than the one that just failed.
	s.autoTried = make(map[string]bool)
	for _, cand := range ranked {
		if cand.ID != failedID {
			s.autoTried[cand.ID] = true
			return cand, true
		}
	}
	s.autoTried[ranked[0].ID] = true
	return ranked[0], true
}
