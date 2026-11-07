package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/z-chenhao/assistant/internal/agent/sqlite"
	"github.com/z-chenhao/assistant/internal/connectedapps"
	"github.com/z-chenhao/assistant/internal/connector/composio"
	"github.com/z-chenhao/assistant/internal/interaction"
	"github.com/z-chenhao/assistant/internal/llm/memory"
)

// The maintained browser gate opts in after Playwright installation. Ordinary
// Go control-plane tests need neither a browser nor a live external account.
func TestAppsBrowserConnectionLifecycle(t *testing.T) {
	if os.Getenv("STEWARD_TEST_APP_BROWSER") != "1" {
		t.Skip("run by make test-browser")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	store, err := sqlite.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s := languageServer()
	s.Core.(*updatesCore).Core = &notificationCore{preference: interaction.Preference{Channel: "auto"}}
	s.Models = settingsModels{}
	s.Capabilities = settingsLayoutCapabilities{}
	s.Tasks = workspaceReader{}
	s.Schedules = &scheduleServiceFixture{}
	var mu sync.Mutex
	status := map[string]string{}
	creations := 0
	secrets := memory.New()
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/test/configure-project" {
			if err := secrets.Put(r.Context(), connectedapps.DefaultKeyRef, []byte("browser-test-project")); err != nil {
				t.Error(err)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/test/activate" {
			status[r.URL.Query().Get("id")] = "ACTIVE"
			w.WriteHeader(204)
			return
		}
		if r.URL.Path == "/test/expire" {
			status[r.URL.Query().Get("id")] = "EXPIRED"
			w.WriteHeader(204)
			return
		}
		if r.URL.Path == "/test/count" {
			_ = json.NewEncoder(w).Encode(map[string]int{"created": creations})
			return
		}
		switch r.URL.Path {
		case "/toolkits":
			_, _ = w.Write([]byte(`{"items":[{"slug":"gmail","name":"Gmail","meta":{"description":"Connect your mail account."}}]}`))
		case "/toolkits/gmail":
			_, _ = w.Write([]byte(`{"slug":"gmail","name":"Gmail"}`))
		case "/auth_configs":
			_, _ = w.Write([]byte(`{"items":[]}`))
		case "/tool_router/session":
			_, _ = w.Write([]byte(`{"session_id":"trs_browser"}`))
		case "/tool_router/session/trs_browser/link":
			creations++
			id := "ca_" + strconv.Itoa(creations)
			status[id] = "INITIALIZING"
			_ = json.NewEncoder(w).Encode(map[string]string{"redirect_url": "https://app.composio.dev/link/" + id, "connected_account_id": id})
		default:
			if strings.HasPrefix(r.URL.Path, "/connected_accounts/") {
				id := strings.TrimPrefix(r.URL.Path, "/connected_accounts/")
				if strings.HasSuffix(id, "/revoke") {
					id = strings.TrimSuffix(id, "/revoke")
					status[id] = "REVOKED"
					_ = json.NewEncoder(w).Encode(map[string]any{"connected_account": map[string]string{"id": id, "status": "REVOKED"}})
					return
				}
				if r.Method == "DELETE" {
					delete(status, id)
					w.WriteHeader(204)
					return
				}
				state, _ := store.LoadApps(r.Context())
				_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "user_id": state.UserID, "status": status[id], "toolkit": map[string]string{"slug": "gmail"}, "data": map[string]string{"access_token": "never-expose"}})
				return
			}
			t.Error("unexpected broker request", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer peer.Close()
	s.Apps = &connectedapps.Service{Store: store, Secrets: secrets, Client: composio.Client{Endpoint: peer.URL}}
	server := httptest.NewServer(s)
	defer server.Close()
	script, err := filepath.Abs("../../../tests/system/apps_connections_browser.mjs")
	if err != nil {
		t.Fatal(err)
	}
	dir := os.Getenv("STEWARD_APP_BROWSER_EVIDENCE")
	if dir == "" {
		dir = t.TempDir()
	}
	cmd := exec.CommandContext(ctx, "node", script, server.URL, peer.URL, dir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("browser flow: %v\n%s", err, output)
	}
}

// Read-only domain peer; rendering and Select enhancement use the real server.
type settingsLayoutCapabilities struct{}

func (settingsLayoutCapabilities) List(context.Context) ([]Capability, error) {
	return []Capability{{ID: "memory", Title: "Memory", State: "Enabled", Automatic: true}}, nil
}
func (settingsLayoutCapabilities) Detail(ctx context.Context, _ string) (Capability, error) {
	items, err := (settingsLayoutCapabilities{}).List(ctx)
	return items[0], err
}
func (settingsLayoutCapabilities) Test(context.Context, string, string, string) (Capability, error) {
	return Capability{}, ErrCapabilityUnavailable
}
func (settingsLayoutCapabilities) Save(context.Context, string, string, string) error {
	return ErrCapabilityUnavailable
}
