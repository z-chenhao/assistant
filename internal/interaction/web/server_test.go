package web

import (
	"context"
	"io"
	"net"
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

func TestEmbeddedAssetsKeepPublicRoutesAndRequestGuards(t *testing.T) {
	s := &Server{Config: Config{TokenRef: "credential:test"}, Core: identityCore{}}
	for _, tc := range []struct {
		method, path, origin, host, contentType string
		status                                  int
	}{
		{"GET", "/app.js", "", "", "text/javascript", 200},
		{"GET", "/appearance.js", "", "", "text/javascript", 200},
		{"GET", "/style.css", "", "", "text/css", 200},
		{"GET", "/favicon.png", "", "", "image/png", 200},
		{"GET", "/brand/ghost-charcoal.png", "", "", "image/png", 200},
		{"GET", "/brand/SourceHanSansSC-common.woff2", "", "", "font/woff2", 200},
		{"GET", "/licenses.txt", "", "", "text/plain; charset=utf-8", 200},
		{"POST", "/style.css", "", "", "text/css", 200},
		{"HEAD", "/style.css", "", "", "", 405},
		{"GET", "/style.css", "", "evil.example:8080", "", 403},
		{"POST", "/style.css", "http://evil.example", "", "", 403},
		{"GET", "/brand/SourceHanSansSC-missing.woff2", "", "", "", 404},
		{"GET", "/brand/SourceHanSansSC-../SourceHanSansSC-common.woff2", "", "", "", 404},
		{"GET", "/brand/unknown.png", "", "", "", 401},
	} {
		t.Run(tc.method+tc.path+tc.origin+tc.host, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "http://127.0.0.1:8080"+tc.path, nil)
			if tc.host != "" {
				r.Host = tc.host
			}
			r.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d", w.Code, tc.status)
			}
			if tc.contentType != "" && (w.Header().Get("Content-Type") != tc.contentType || w.Body.Len() == 0) {
				t.Fatal("missing asset or changed media type", w.Header(), w.Body.Len())
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'none'") {
				t.Fatal("asset request bypassed response guards", w.Header())
			}
		})
	}
}

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

func TestOptionalTokenConfigKeepsLiteralLoopbackBoundary(t *testing.T) {
	for _, config := range []Config{{Listen: "127.0.0.1:8787"}, {Listen: "[::1]:8787"}, {Listen: "127.0.0.1:8787", TokenRef: "env:WEB_TOKEN"}} {
		if err := config.Validate(); err != nil {
			t.Fatal(config, err)
		}
	}
	for _, config := range []Config{{Listen: "0.0.0.0:8787"}, {Listen: "localhost:8787"}, {Listen: "127.0.0.1:8787", TokenRef: "literal-secret"}} {
		if config.Validate() == nil {
			t.Fatal("unsafe config accepted", config)
		}
	}
}

func TestLocalWorkspaceWithoutTokenUsesRealLoopbackAndKeepsRequestGuards(t *testing.T) {
	core := &updatesCore{allowed: true, status: "idle"}
	workers := &workerReader{}
	s := &Server{Core: core, Workers: workers}
	host := httptest.NewServer(s)
	defer host.Close()
	client := host.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	request := func(method, path, origin, site, hostname string) *http.Response {
		r, err := http.NewRequest(method, host.URL+path, strings.NewReader("language=zh-CN"))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if site != "" {
			r.Header.Set("Sec-Fetch-Site", site)
		}
		if hostname != "" {
			r.Host = hostname
		}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	r := request("GET", "/", "", "none", "")
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != 200 || strings.Contains(string(body), "Workspace access token") || !strings.Contains(string(body), `class="chat-identity"`) {
		t.Fatal(r.StatusCode, string(body))
	}
	if len(r.Cookies()) != 0 {
		t.Fatal("direct access created an auth session")
	}
	r = request("GET", "/api/workers", "", "same-origin", "")
	r.Body.Close()
	if r.StatusCode != 200 || workers.calls != 1 {
		t.Fatal(r.StatusCode, workers.calls)
	}
	r = request("POST", "/settings/language", host.URL, "same-origin", "")
	r.Body.Close()
	if r.StatusCode != 303 || len(r.Cookies()) != 1 || r.Cookies()[0].Name != languageCookie {
		t.Fatal("same-origin form failed", r.StatusCode, r.Cookies())
	}
	for _, test := range []struct{ method, origin, site, host string }{{"GET", "http://evil.example", "cross-site", ""}, {"GET", "", "cross-site", ""}, {"GET", "", "same-site", ""}, {"POST", "http://127.0.0.1:1", "same-site", ""}, {"GET", "", "", "evil.example:8787"}} {
		r = request(test.method, "/api/workers", test.origin, test.site, test.host)
		r.Body.Close()
		if r.StatusCode != 403 {
			t.Fatal("untrusted request accepted", test, r.StatusCode)
		}
	}
	forged := httptest.NewRequest("GET", host.URL+"/api/workers", nil)
	forged.RemoteAddr = "192.0.2.5:8080"
	forged.Header.Set("X-Forwarded-For", "127.0.0.1")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, forged)
	if w.Code != 403 || workers.calls != 1 {
		t.Fatal("forwarded peer trusted", w.Code, workers.calls)
	}
	core.allowed = false
	r = request("GET", "/api/workers", "", "same-origin", "")
	r.Body.Close()
	if r.StatusCode != 403 || workers.calls != 1 {
		t.Fatal("binding revocation bypassed", r.StatusCode, workers.calls)
	}
}

type nonLoopbackListener struct{ net.Listener }

func (nonLoopbackListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4zero, Port: 8787} }
func TestTokenFreeServeRejectsNonLoopbackListener(t *testing.T) {
	if err := (&Server{}).Serve(context.Background(), nonLoopbackListener{}); err == nil {
		t.Fatal("token-free public listener accepted")
	}
}
