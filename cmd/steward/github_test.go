package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/z-chenhao/assistant/internal/observer/github"
)

type githubPeerTransport struct {
	peer       *url.URL
	underlying http.RoundTripper
}

func (p githubPeerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "https" || r.URL.Host != "api.github.com" {
		return nil, errors.New("unexpected external endpoint")
	}
	clone := r.Clone(r.Context())
	clone.URL.Scheme = p.peer.Scheme
	clone.URL.Host = p.peer.Host
	return p.underlying.RoundTrip(clone)
}

func TestGitHubCLISuccessfulPollPersistsDedupAndFailureBoundary(t *testing.T) {
	root := t.TempDir()
	t.Setenv("STEWARD_DATA_ROOT", filepath.Join(root, "data"))
	t.Setenv("GITHUB_OBSERVER_SYNTHETIC_TEST", "synthetic-observer-token")
	config := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(config, []byte("version: 1\ngithub:\n  repository: owner/repo\n  pull_number: 7\n  credential_ref: env:GITHUB_OBSERVER_SYNTHETIC_TEST\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var fail atomic.Bool
	var revision atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer synthetic-observer-token" {
			t.Error("wrong authentication boundary")
		}
		switch r.URL.Path {
		case "/user":
			fmt.Fprint(w, `{"id":1,"login":"steward"}`)
		case "/repos/owner/repo/pulls/7":
			fmt.Fprint(w, `{"number":7,"head":{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"base":{"repo":{"id":99,"full_name":"owner/repo"}}}`)
		case "/repos/owner/repo/pulls/7/comments":
			fmt.Fprintf(w, `[{"id":5,"body":"requested change %d","updated_at":"2026-11-01T00:00:00Z","html_url":"https://github.com/owner/repo/pull/7#discussion_r5","user":{"id":2,"login":"reviewer"}}]`, revision.Load())
		case "/repos/owner/repo/pulls/7/reviews":
			if fail.Load() {
				http.Error(w, "synthetic private provider failure", http.StatusServiceUnavailable)
				return
			}
			fmt.Fprint(w, `[]`)
		case "/repos/owner/repo/commits/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/check-runs":
			fmt.Fprint(w, `{"check_runs":[]}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	prior := http.DefaultTransport
	http.DefaultTransport = githubPeerTransport{u, prior}
	defer func() { http.DefaultTransport = prior }()
	invoke := func(op string) ([]byte, error) {
		t.Helper()
		var out, errOut bytes.Buffer
		err := run(context.Background(), []string{"-config", config, "github", op}, strings.NewReader(""), &out, &errOut)
		return out.Bytes(), err
	}
	events := func(want int) {
		t.Helper()
		raw, err := invoke("events")
		var events []github.Observation
		if err != nil || json.Unmarshal(raw, &events) != nil || len(events) != want {
			t.Fatalf("events: %s %v", raw, err)
		}
	}
	for range 2 {
		if _, err = invoke("poll"); err != nil {
			t.Fatal(err)
		}
	}
	events(1)
	revision.Store(1)
	fail.Store(true)
	if _, err = invoke("poll"); err == nil || strings.Contains(err.Error(), "private provider") {
		t.Fatal("unsafe scan failure", err)
	}
	events(1)
	fail.Store(false)
	if _, err = invoke("poll"); err != nil {
		t.Fatal(err)
	}
	events(2)
	db, err := sql.Open("sqlite", "file:"+filepath.Join(root, "data", "state.db")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"run", "main_input", "task"} {
		var n int
		if err = db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatal("observation created unauthorized work", table, n, err)
		}
	}
}

func TestGitHubCLIControlPersistsWithoutExecutionReadiness(t *testing.T) {
	root := t.TempDir()
	t.Setenv("STEWARD_DATA_ROOT", filepath.Join(root, "data"))
	t.Setenv("GITHUB_OBSERVER_MISSING_TEST", "")
	config := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(config, []byte("version: 1\njavascript:\n  command: /missing/node\n  script: /missing/script.mjs\ngithub:\n  repository: owner/repo\n  pull_number: 7\n  credential_ref: env:GITHUB_OBSERVER_MISSING_TEST\n"), 0600); err != nil {
		t.Fatal(err)
	}
	invoke := func(operation string) ([]byte, error) {
		t.Helper()
		var out, errOut bytes.Buffer
		err := run(context.Background(), []string{"-config", config, "github", operation}, strings.NewReader(""), &out, &errOut)
		return out.Bytes(), err
	}
	status := func(want string) {
		t.Helper()
		raw, err := invoke("status")
		var v github.Status
		if err != nil || json.Unmarshal(raw, &v) != nil || v.State != want {
			t.Fatalf("status %s: %s %v", want, raw, err)
		}
	}
	status("never_checked")
	if raw, err := invoke("binding"); err != nil || !strings.Contains(string(raw), `"state":"unbound"`) {
		t.Fatal("binding control depended on execution runtime", string(raw), err)
	}
	if _, err := invoke("poll"); err == nil || !strings.Contains(err.Error(), "CREDENTIAL_MISSING") {
		t.Fatal("missing credential accepted", err)
	}
	status("interrupted")
	if _, err := invoke("stop"); err != nil {
		t.Fatal(err)
	}
	status("stopped")
	if _, err := invoke("poll"); err == nil || !strings.Contains(err.Error(), "stopped") {
		t.Fatal("stopped watch revived", err)
	}
	if raw, err := invoke("events"); err != nil || strings.TrimSpace(string(raw)) != "[]" {
		t.Fatal(string(raw), err)
	}
	if _, err := invoke("resume"); err != nil {
		t.Fatal(err)
	}
	status("never_checked")
	// Removing the source from the next process must not project its old scope
	// as active or leave its observations reachable through the current config.
	if err := os.WriteFile(config, []byte("version: 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	status("not_configured")
	if _, err := invoke("events"); err == nil {
		t.Fatal("removed source remained accessible")
	}
}
