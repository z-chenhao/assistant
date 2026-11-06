package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/agent/sqlite"
	"github.com/z-chenhao/assistant/internal/connectedapps"
	"github.com/z-chenhao/assistant/internal/connector/composio"
)

func TestAppsOfflineDirectoryAndOwnerConnectionFlow(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s := languageServer()
	service := &connectedapps.Service{Store: store}
	s.Apps = service
	w := httptest.NewRecorder()
	s.ServeHTTP(w, languageRequest("GET", "/settings/apps", "zh-CN", ""))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Gmail") || strings.Contains(w.Body.String(), "应用连接服务尚未配置") || strings.Contains(w.Body.String(), `value="connect"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "composio-key-ref") || strings.Contains(w.Body.String(), "auth_config") || strings.Contains(w.Body.String(), `href="/settings/services/apps"`) {
		t.Fatal("deployment fields leaked into Apps")
	}
	w = httptest.NewRecorder()
	s.ServeHTTP(w, languageRequest("GET", "/settings/services/apps", "en", ""))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "composio-key-ref") || !strings.Contains(w.Body.String(), `href="/settings/setup" aria-current="page"`) {
		t.Fatal("deployment settings not separate", w.Code, w.Body.String())
	}
	for _, request := range []struct{ method, path, body string }{{"GET", "/settings/apps?search=notion", ""}, {"GET", "/settings/apps?category=communication", ""}} {
		w = httptest.NewRecorder()
		s.ServeHTTP(w, languageRequest(request.method, request.path, "en", request.body))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if request.path == "/settings/apps?search=notion" && (strings.Contains(w.Body.String(), "<strong>Gmail</strong>") || !strings.Contains(w.Body.String(), "Notion")) {
			t.Fatal("suggestion filtering")
		}
	}
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth_configs":
			_, _ = w.Write([]byte(`{"items":[]}`))
		case "/toolkits":
			_, _ = w.Write([]byte(`{"items":[{"slug":"gmail","name":"Gmail","meta":{"description":"Mail <script>injection</script>"}}],"next_cursor":"page+2=&","total_items":1700}`))
		case "/toolkits/gmail":
			_, _ = w.Write([]byte(`{"slug":"gmail","name":"Gmail"}`))
		case "/tool_router/session":
			_, _ = w.Write([]byte(`{"session_id":"trs_test"}`))
		case "/tool_router/session/trs_test/link":
			_, _ = w.Write([]byte(`{"redirect_url":"https://app.composio.dev/link/private-link","connected_account_id":"ca_test"}`))
		case "/connected_accounts/ca_test":
			if r.Method == "DELETE" {
				w.WriteHeader(204)
				return
			}
			state, _ := store.LoadApps(r.Context())
			_, _ = w.Write([]byte(`{"id":"ca_test","user_id":"` + state.UserID + `","status":"ACTIVE","toolkit":{"slug":"gmail"},"data":{"access_token":"never-expose"}}`))
		case "/connected_accounts/ca_test/revoke":
			_, _ = w.Write([]byte(`{"connected_account":{"id":"ca_test","status":"REVOKED"}}`))
		default:
			t.Error(r.URL)
			http.NotFound(w, r)
		}
	}))
	defer peer.Close()
	service.Client = composio.Client{Endpoint: peer.URL}
	service.Secrets = fakeSecrets{}
	post := func(form url.Values) *httptest.ResponseRecorder {
		t.Helper()
		result := httptest.NewRecorder()
		s.ServeHTTP(result, languageRequest("POST", "/apps/configure", "en", form.Encode()))
		return result
	}
	w = httptest.NewRecorder()
	s.ServeHTTP(w, languageRequest("POST", "/apps/service/configure", "en", url.Values{"revision": {"1"}, "key_ref": {"env:COMPOSIO_API_KEY"}}.Encode()))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	s.ServeHTTP(w, languageRequest("GET", "/settings/apps", "en", ""))
	if strings.Contains(w.Body.String(), "<script>injection</script>") || !strings.Contains(w.Body.String(), "page%2b2%3d%26") || strings.Contains(w.Body.String(), "env:COMPOSIO_API_KEY") {
		t.Fatal(w.Body.String())
	}
	w = post(url.Values{"action": {"connect"}, "toolkit": {"gmail"}, "alias": {"work"}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "https://app.composio.dev/link/private-link") || w.Header().Get("Referrer-Policy") != "no-referrer" || !strings.Contains(w.Body.String(), "pending") {
		t.Fatal(w.Code, w.Body.String())
	}
	state, _ := store.LoadApps(ctx)
	id := state.Connections[0].ID
	w = post(url.Values{"action": {"check"}, "connection": {id}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "active") || strings.Contains(w.Body.String(), "never-expose") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = post(url.Values{"action": {"disconnect"}, "connection": {id}})
	if w.Code != 200 || strings.Contains(w.Body.String(), `data-app-account="`+id+`"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	state, _ = store.LoadApps(ctx)
	if !state.Connections[0].RevocationConfirmed {
		t.Fatal("revocation not confirmed")
	}
	r := languageRequest("POST", "/apps/configure", "en", "action=connect&toolkit=gmail&alias=personal")
	r.Header.Set("Origin", "https://attacker.example")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-site connection allowed", w.Code)
	}
	r = languageRequest("POST", "/apps/configure", "en", "action=connect&toolkit=gmail&alias=personal")
	r.Header.Del("Authorization")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code == 200 {
		t.Fatal("unauthenticated connection allowed")
	}
}
