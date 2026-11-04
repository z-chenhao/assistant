package web

import (
	"context"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/agent"
)

type richFormCore struct {
	updatesCore
	calls         int
	message, form string
	values        map[string]string
}

func (c *richFormCore) RespondWebForm(_ context.Context, message, form string, values map[string]string) (agent.Receipt, error) {
	c.calls++
	c.message, c.form, c.values = message, form, values
	return agent.Receipt{MessageID: "accepted"}, nil
}

func TestRichFormHTTPAcceptsOnlyAnswersWithinExistingAccessBoundary(t *testing.T) {
	good := url.Values{"message_id": {"reply:one"}, "form_id": {strings.Repeat("a", 64)}, "field.city": {"上海"}}
	for _, scenario := range []string{"valid", "native", "unauthenticated", "cross-origin", "revoked", "extra action", "duplicate identity", "query identity"} {
		t.Run(scenario, func(t *testing.T) {
			core := &richFormCore{updatesCore: updatesCore{allowed: true}}
			s := &Server{Core: core, Config: Config{TokenRef: "credential:test"}, Secrets: fakeSecrets{}}
			values := url.Values{}
			for k, v := range good {
				values[k] = append([]string(nil), v...)
			}
			want := 200
			if scenario == "extra action" {
				values.Set("approval_id", "forged")
				want = 400
			}
			if scenario == "duplicate identity" {
				values.Add("form_id", "different")
				want = 400
			}
			path := "/ui/form"
			if scenario == "query identity" {
				delete(values, "message_id")
				path += "?message_id=reply:forged"
				want = 400
			}
			r := languageRequest("POST", path, "", values.Encode())
			r.Header.Set("Accept", "application/json")
			if scenario == "unauthenticated" {
				r.Header.Del("Authorization")
				want = 401
			}
			if scenario == "cross-origin" {
				r.Header.Set("Origin", "https://other.example")
				want = 403
			}
			if scenario == "revoked" {
				core.allowed = false
				want = 403
			}
			if scenario == "native" {
				r.Header.Del("Accept")
				want = 303
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != want {
				t.Fatal(w.Code, w.Body.String())
			}
			if want != 200 && want != 303 {
				if core.calls != 0 {
					t.Fatal("invalid form reached admission")
				}
				return
			}
			if core.calls != 1 || core.form != good.Get("form_id") || !reflect.DeepEqual(core.values, map[string]string{"city": "上海"}) {
				t.Fatal(core)
			}
			if core.message != "reply:one" {
				t.Fatal(core.message)
			}
		})
	}
}
