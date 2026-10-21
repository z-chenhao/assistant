package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeSecrets struct{}

func (fakeSecrets) Get(context.Context, string) ([]byte, error) {
	return []byte("private-token-at-least-24-bytes"), nil
}
func (fakeSecrets) Put(context.Context, string, []byte) error { return nil }
func (fakeSecrets) Delete(context.Context, string) error      { return nil }

type identityCore struct{ Core }

func (identityCore) CheckChannel(context.Context, string, string, string) error { return nil }
func TestAuthenticationCSRFRebindingSessionExpiryAndSafeErrors(t *testing.T) {
	s := &Server{Config: Config{TokenRef: "credential:test"}, Secrets: fakeSecrets{}, Core: identityCore{}}
	request := func(method, path, token, origin, host string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, strings.NewReader("token="+token))
		if host != "" {
			r.Host = host
		}
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/api/conversation", "", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request("POST", "/session", "private-token-at-least-24-bytes", "http://evil.example", ""); w.Code != 403 {
		t.Fatal("CSRF", w.Code)
	}
	if w := request("POST", "/session", "private-token-at-least-24-bytes", "", "evil.example:8080"); w.Code != 403 {
		t.Fatal("DNS rebinding", w.Code)
	}
	if w := request("POST", "/session", "wrong", "", ""); w.Code != 401 || strings.Contains(w.Body.String(), "private-token") {
		t.Fatal("auth rejection", w.Code, w.Body.String())
	}
	w := request("POST", "/session", "private-token-at-least-24-bytes", "", "")
	if w.Code != 303 {
		t.Fatal(w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("insecure session", cookies)
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/unknown", nil)
	r.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal("session unusable", w.Code)
	}
	s.sessionExpires = time.Now().Add(-time.Second)
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("expired session", w.Code)
	}
	if AuthURL("javascript:alert(1)") != "" || AuthURL("http://untrusted") != "" {
		t.Fatal("unsafe auth URL")
	}
}
