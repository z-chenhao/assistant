package web

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/z-chenhao/collie/internal/interaction"
	"github.com/z-chenhao/collie/internal/llm"
)

type notificationCore struct {
	Core
	preference interaction.Preference
	bindings   []interaction.Binding
}

func (c *notificationCore) CheckChannel(context.Context, string, string, string) error { return nil }
func (c *notificationCore) WebStatus(context.Context) (Status, error) {
	return Status{Readiness: "ready", Status: "idle"}, nil
}
func (c *notificationCore) ChannelPreference(context.Context) (interaction.Preference, error) {
	return c.preference, nil
}
func (c *notificationCore) ChannelBindings(context.Context) ([]interaction.Binding, error) {
	return c.bindings, nil
}
func (c *notificationCore) PreferChannel(_ context.Context, ch, b string, v int64) error {
	if v != c.preference.Version {
		return llm.ErrConflict
	}
	c.preference.Channel = ch
	c.preference.BindingID = nil
	if b != "" {
		c.preference.BindingID = &b
	}
	c.preference.Version++
	return nil
}

func TestNotificationSaveStaysInItsPageAndReflectsCanonicalSelection(t *testing.T) {
	core := &notificationCore{preference: interaction.Preference{Channel: "auto", Version: 3}}
	s := &Server{Core: core, Secrets: fakeSecrets{}}
	req := httptest.NewRequest("POST", "http://127.0.0.1:8080/notification", strings.NewReader(url.Values{"target": {"tui:"}, "version": {"3"}}.Encode()))
	req.Header.Set("Authorization", "Bearer private-token-at-least-24-bytes")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	saved := httptest.NewRecorder()
	s.ServeHTTP(saved, req)
	if saved.Code != 303 || saved.Header().Get("Location") != "/settings/notifications?notice=saved" {
		t.Fatal(saved.Code, saved.Header())
	}
	read := httptest.NewRequest("GET", "http://127.0.0.1:8080/settings/notifications", nil)
	read.Header.Set("Authorization", "Bearer private-token-at-least-24-bytes")
	page := httptest.NewRecorder()
	s.ServeHTTP(page, read)
	if page.Code != 200 || !strings.Contains(page.Body.String(), `value="tui:" selected`) {
		t.Fatal("saved preference reverted to default in the form", page.Code, page.Body.String())
	}
	id := "missing"
	core.preference = interaction.Preference{Channel: "feishu", BindingID: &id, Version: 4}
	page = httptest.NewRecorder()
	s.ServeHTTP(page, read)
	if !strings.Contains(page.Body.String(), "Your saved destination is no longer available") || !strings.Contains(page.Body.String(), `value="" selected`) {
		t.Fatal("unavailable binding silently selected Automatic", page.Body.String())
	}
}

type candidateProof struct {
	llm.ProfileStore
	record llm.ProbeRecord
}

func (p candidateProof) Probe(context.Context, string) (llm.ProbeRecord, error) { return p.record, nil }
func TestCandidateVerificationIsDistinctFromActivation(t *testing.T) {
	profile := llm.Profile{ProfileID: "candidate", Status: "unverified", Version: 7, ProfileInput: llm.ProfileInput{Capabilities: llm.Capabilities{TextInput: true}}}
	record := llm.ProbeRecord{ProfileVersion: 7, BridgeIdentity: "bridge-v1", Result: llm.ProbeResult{Streaming: true, ToolCalls: true, Continuation: true, Cancellation: true, Clipping: true}}
	s := &Server{Control: llm.Control{Store: candidateProof{record: record}, BridgeIdentity: "bridge-v1"}}
	if !s.profileVerified(context.Background(), profile) {
		t.Fatal("passed candidate cannot reach explicit activation")
	}
	s.Control.BridgeIdentity = "changed-bridge"
	if s.profileVerified(context.Background(), profile) {
		t.Fatal("stale proof shown as verified")
	}
	s.Control.BridgeIdentity = "bridge-v1"
	profile.Version++
	if s.profileVerified(context.Background(), profile) {
		t.Fatal("changed candidate reused a stale probe")
	}
}

func TestWorkspaceLoginReturnIsLocalAndPreservesDeepLinks(t *testing.T) {
	for _, bad := range []string{"https://evil.example/", "//evil.example/", "javascript:alert(1)", "/api/status", "/settings/models/../../api/status", "/tasks/task:bad/child", "/settings/models/%0d%0aLocation:evil"} {
		if got := safePageReturn(bad); got != "/" {
			t.Fatal("unsafe return", bad, got)
		}
	}
	valid := "/tasks/task:visible?history=cursor"
	if safePageReturn(valid) != valid {
		t.Fatal("lost task deep link")
	}
	s := &Server{Secrets: fakeSecrets{}}
	r := httptest.NewRequest("POST", "http://127.0.0.1:8080/session", strings.NewReader(url.Values{"token": {"private-token-at-least-24-bytes"}, "next": {valid}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 303 || w.Header().Get("Location") != valid {
		t.Fatal("login discarded the original destination", w.Code, w.Header())
	}
}
