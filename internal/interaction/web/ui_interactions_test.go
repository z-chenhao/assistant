package web

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/delivery/ui"
)

type uiStateCore struct {
	updatesCore
	reads, writes, actions int
}

func (c *uiStateCore) WebUIState(context.Context, string) (ui.ViewState, error) {
	c.reads++
	return ui.ViewState{State: json.RawMessage(`{}`), ModelContext: json.RawMessage(`{}`)}, nil
}
func (c *uiStateCore) SaveWebUIState(_ context.Context, _ string, state ui.ViewState) (ui.ViewState, error) {
	c.writes++
	state.Version++
	return state, nil
}
func (c *uiStateCore) RequestWebUIAction(context.Context, string, string) (agent.Receipt, error) {
	c.actions++
	return agent.Receipt{MessageID: "accepted"}, nil
}
func (c *uiStateCore) WebUIActionResult(context.Context, string, string) (ui.ActionResult, error) {
	c.reads++
	return ui.ActionResult{Status: "pending"}, nil
}
func TestUIInteractionHTTPKeepsAuthenticationCSRFAndStrictInputs(t *testing.T) {
	for _, scenario := range []string{"read", "save", "action", "unauthorized", "cross-origin", "revoked", "forged-tool", "trailing-json", "state-action-fields"} {
		t.Run(scenario, func(t *testing.T) {
			core := &uiStateCore{updatesCore: updatesCore{allowed: true}}
			server := &Server{Core: core, Config: Config{TokenRef: "credential:test"}, Secrets: fakeSecrets{}}
			method, path, body := "POST", "/api/ui/state", `{"ref":"ui:accepted#0","state":{"value":42},"model_context":{}}`
			want := 200
			switch scenario {
			case "read":
				method, path, body = "GET", "/api/ui/state?ref=ui:accepted%230", ""
			case "action":
				path, body = "/api/ui/action", `{"ref":"ui:accepted#1","source_id":"stable-action-identity"}`
			case "forged-tool":
				path, body, want = "/api/ui/action", `{"ref":"ui:accepted#1","source_id":"stable-action-identity","tool":"mcp__forged__write"}`, 400
			case "trailing-json":
				body += `{}`
				want = 400
			case "state-action-fields":
				body = `{"ref":"ui:accepted#0","state":{},"model_context":{},"source_id":"forged"}`
				want = 400
			case "revoked":
				core.allowed = false
				want = 403
			case "cross-origin":
				want = 403
			case "unauthorized":
				want = 401
			}
			request := languageRequest(method, path, "", body)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept", "application/json")
			if scenario == "unauthorized" {
				request.Header.Del("Authorization")
			}
			if scenario == "cross-origin" {
				request.Header.Set("Origin", "https://untrusted.example")
			}
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != want {
				t.Fatal(response.Code, response.Body.String())
			}
			if want != 200 && (core.reads+core.writes+core.actions) > 0 {
				t.Fatal("invalid request crossed the Host boundary")
			}
			if scenario == "read" && (core.reads != 1 || core.writes != 0 || core.actions != 0) {
				t.Fatal("GET mutated UI state")
			}
			if want == 200 && !strings.Contains(response.Header().Get("Content-Type"), "application/json") {
				t.Fatal("non JSON response")
			}
		})
	}
}
