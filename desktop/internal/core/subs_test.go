package core

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

const subVLESS = "vless://u1@example.com:443?security=reality&sni=a.com&pbk=k1&sid=01&type=tcp#Node1"
const subHy2 = "hysteria2://pw@down.example:39452?sni=b.com#Node2"

func TestParseSubscriptionBase64(t *testing.T) {
	body := base64.StdEncoding.EncodeToString([]byte(subVLESS + "\n" + subHy2 + "\n"))
	res, err := ParseSubscriptionBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Profiles) != 2 || res.Skipped != 0 {
		t.Fatalf("got %d profiles, skipped %d", len(res.Profiles), res.Skipped)
	}
	if res.Profiles[0].Type != TVLESS || res.Profiles[0].Name != "Node1" {
		t.Fatalf("first: %+v", res.Profiles[0])
	}
	if res.Profiles[1].Type != THysteria2 || res.Profiles[1].Host != "down.example" {
		t.Fatalf("second: %+v", res.Profiles[1])
	}
}

func TestParseSubscriptionPlainAndJunk(t *testing.T) {
	body := strings.Join([]string{
		subVLESS,
		"# comment",
		"",
		"   ",
		"not a link at all",
		subHy2,
	}, "\n")
	res, err := ParseSubscriptionBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Profiles) != 2 || res.Skipped != 1 {
		t.Fatalf("got %d profiles, skipped %d", len(res.Profiles), res.Skipped)
	}
}

func TestParseSubscriptionURLSafeBase64(t *testing.T) {
	body := base64.URLEncoding.EncodeToString([]byte(subVLESS))
	res, err := ParseSubscriptionBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Profiles) != 1 || res.Profiles[0].Type != TVLESS {
		t.Fatalf("got %d profiles", len(res.Profiles))
	}
}

func TestParseSubscriptionHTMLError(t *testing.T) {
	if _, err := ParseSubscriptionBody("<!DOCTYPE html>\n<html><body>gateway error</body></html>\n"); err == nil {
		t.Fatal("expected an error for an HTML body")
	}
	if _, err := ParseSubscriptionBody("just garbage\nmore garbage\n"); err == nil {
		t.Fatal("expected an error for a body without links")
	}
}

func TestSubscriptionStoreLifecycle(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sub, err := s.AddSubscription("", "https://subs.example.com/list")
	if err != nil {
		t.Fatal(err)
	}
	if sub.Name != "subs.example.com" {
		t.Fatalf("default name = %q, want host", sub.Name)
	}
	// Same URL again returns the existing subscription.
	dup, err := s.AddSubscription("other", "https://subs.example.com/list")
	if err != nil || dup.ID != sub.ID {
		t.Fatalf("dup: %v %+v", err, dup)
	}
	// Non-http URL is rejected.
	if _, err := s.AddSubscription("", "wisteria://x"); err == nil {
		t.Fatal("expected rejection of a non-http URL")
	}

	// First refresh: two nodes.
	first, err := ParseSubscriptionBody(subVLESS + "\n" + subHy2 + "\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReplaceSubscriptionProfiles(sub.ID, first.Profiles); err != nil {
		t.Fatal(err)
	}
	ps := s.List()
	if len(ps) != 2 {
		t.Fatalf("profiles = %d", len(ps))
	}
	var vlessID string
	for _, p := range ps {
		if p.Type == TVLESS {
			vlessID = p.ID
		}
	}
	if vlessID == "" {
		t.Fatal("vless profile missing after first refresh")
	}
	s.SetLatency(vlessID, 42)

	sub.Count = len(first.Profiles)
	sub.UpdatedAt = time.Now()
	if err := s.UpdateSubscription(sub); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSubscription(sub.ID)
	if err != nil || got.Count != 2 || got.UpdatedAt.IsZero() {
		t.Fatalf("sub after update: %+v err=%v", got, err)
	}

	// Second refresh: node1 keeps id/latency but gets the new name; node2
	// is gone; a new node appears. A manual profile at the same endpoint
	// must NOT be swallowed.
	manual := Profile{Name: "manual", Type: TVLESS, Host: "example.com", Port: 443,
		Fields: map[string]string{"uuid": "manual-uuid"}}
	if _, err := s.Add(manual); err != nil {
		t.Fatal(err)
	}
	renamed := strings.Replace(subVLESS, "#Node1", "#Node1-renamed", 1)
	second, err := ParseSubscriptionBody(renamed + "\n" + subHy2NewNode + "\n")
	if err != nil {
		t.Fatal(err)
	}
	added, err := s.ReplaceSubscriptionProfiles(sub.ID, second.Profiles)
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}
	ps = s.List()
	if len(ps) != 3 { // node1 (reused) + new sub node + manual
		t.Fatalf("profiles = %d: %+v", len(ps), ps)
	}
	var node1 *Profile
	for i := range ps {
		if ps[i].SourceID == sub.ID && ps[i].Name == "Node1-renamed" {
			node1 = &ps[i]
		}
	}
	if node1 == nil {
		t.Fatal("renamed node1 not found")
	}
	if node1.ID != vlessID || node1.LatencyMs != 42 {
		t.Fatalf("identity lost: id=%s latency=%d", node1.ID, node1.LatencyMs)
	}
	if node1.SourceID != sub.ID {
		t.Fatalf("source = %q", node1.SourceID)
	}

	// Delete removes exactly the sub's profiles.
	if err := s.DeleteSubscription(sub.ID); err != nil {
		t.Fatal(err)
	}
	ps = s.List()
	if len(ps) != 1 || ps[0].SourceID != "" {
		t.Fatalf("after delete: %+v", ps)
	}
	if err := s.DeleteSubscription(sub.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double delete: %v", err)
	}
}

const subHy2NewNode = "hysteria2://pw2@new.example:1?sni=c.com#Node3"

func TestSubscriptionWisteriaKeySurvivesRefresh(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sub, err := s.AddSubscription("", "https://subs.example.com/w")
	if err != nil {
		t.Fatal(err)
	}
	pub := base64.StdEncoding.EncodeToString(make([]byte, 32))
	p1, err := ParseLink("wisteria://" + pub + "@srv.example:8443#W1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReplaceSubscriptionProfiles(sub.ID, []Profile{p1}); err != nil {
		t.Fatal(err)
	}
	first := s.List()[0]
	if first.ClientPriv == "" {
		t.Fatal("client key must be generated for a sub-imported wisteria profile")
	}
	// Refresh with the same endpoint: the key must survive.
	if _, err := s.ReplaceSubscriptionProfiles(sub.ID, []Profile{p1}); err != nil {
		t.Fatal(err)
	}
	second := s.List()[0]
	if second.ID != first.ID || second.ClientPriv != first.ClientPriv {
		t.Fatal("wisteria client key or id lost across refresh")
	}
}

func TestSubscriptionsPersistAcrossReopen(t *testing.T) {
	// Regression: OpenStore restored profiles and the active id but dropped
	// the subscriptions list, so every sub disappeared after a restart.
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sub, err := s.AddSubscription("probe", "https://subs.example.com/persist")
	if err != nil {
		t.Fatal(err)
	}
	body, err := ParseSubscriptionBody(subVLESS + "\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReplaceSubscriptionProfiles(sub.ID, body.Profiles); err != nil {
		t.Fatal(err)
	}

	s2, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s2.GetSubscription(sub.ID)
	if err != nil {
		t.Fatalf("subscription lost after reopen: %v", err)
	}
	if got.Name != "probe" || got.URL != "https://subs.example.com/persist" {
		t.Fatalf("sub metadata corrupted: %+v", got)
	}
	found := false
	for _, p := range s2.List() {
		if p.SourceID == sub.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("sub profiles lost their source id after reopen")
	}
}
