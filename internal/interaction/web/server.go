// Package web presents the authenticated local control and conversation surface.
// HTTP handlers do not implement a second execution or approval lifecycle.
package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/z-chenhao/collie/internal/agent"
	"github.com/z-chenhao/collie/internal/artifact"
	"github.com/z-chenhao/collie/internal/conversation"
	"github.com/z-chenhao/collie/internal/interaction"
	"github.com/z-chenhao/collie/internal/llm"
	"github.com/z-chenhao/collie/internal/llm/account"
	"github.com/z-chenhao/collie/internal/task"
)

type Config struct {
	Listen   string `yaml:"listen"`
	TokenRef string `yaml:"token_ref"`
}

func (c Config) Validate() error {
	host, _, err := net.SplitHostPort(c.Listen)
	if err != nil || (host != "127.0.0.1" && host != "::1") || !interaction.ValidCredentialRef(c.TokenRef) {
		return errors.New("Web requires a literal loopback listener and credential reference")
	}
	return nil
}

const Subject = "browser"
const Destination = "local"

type Status struct {
	Readiness            string  `json:"readiness"`
	Status               string  `json:"status"`
	ConfigVersion        int64   `json:"config_version"`
	ActiveModelProfileID *string `json:"active_model_profile_id,omitempty"`
}
type Core interface {
	WaitForRuntime(context.Context) error
	ChannelBindings(context.Context) ([]interaction.Binding, error)
	ChannelPreference(context.Context) (interaction.Preference, error)
	PreferChannel(context.Context, string, string, int64) error
	WebStatus(context.Context) (Status, error)
	Conversation(context.Context, int64) (conversation.Page, error)
	Activity(context.Context) ([]task.Summary, error)
	WebApprovals(context.Context) ([]interaction.Approval, error)
	AcceptInbound(context.Context, interaction.InboundEvent) (agent.Receipt, error)
	DecideInbound(context.Context, interaction.InboundEvent, string, string, string) error
	CheckChannel(context.Context, string, string, string) error
	ImportChannelArtifact(context.Context, interaction.InboundEvent, string, artifact.Input, io.Reader) (string, error)
	SaveArtifact(context.Context, artifact.Input, io.Reader) (artifact.Descriptor, error)
	OpenArtifact(context.Context, string) (io.ReadCloser, error)
	DescribeArtifact(context.Context, string) (artifact.Descriptor, error)
	WebProjection(context.Context, conversation.Message) (json.RawMessage, error)
}
type Models interface {
	ListProfiles(context.Context) ([]llm.Profile, error)
	AddProfile(context.Context, llm.ProfileInput) (llm.Profile, error)
}
type Server struct {
	Inspector      ExecutionReader
	Contexts       ContextReader
	Traces         TraceReader
	Tasks          TaskReader
	Config         Config
	Core           Core
	Models         Models
	Control        llm.Control
	Accounts       *account.Manager
	Secrets        llm.SecretStore
	AccountCatalog func(context.Context, string) ([]llm.CatalogModel, error)
	Callback       http.Handler
	Wake           func()
	mu             sync.Mutex
	requests       sync.WaitGroup
	stopping       bool
	session        string
	sessionExpires time.Time
	auth           llm.AuthState
	accountID      string
	authCancel     context.CancelFunc
	authDone       chan struct{}
	lifetime       context.Context
}

func (s *Server) Serve(ctx context.Context, l net.Listener) error {
	s.lifetime = ctx
	srv := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(l) }()
	var err error
	select {
	case err = <-served:
		s.mu.Lock()
		s.stopping = true
		s.mu.Unlock()
		_ = srv.Close()
	case <-ctx.Done():
		s.mu.Lock()
		s.stopping = true
		s.mu.Unlock()
		stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if shutdownErr := srv.Shutdown(stop); shutdownErr != nil {
			_ = srv.Close()
		}
		cancel()
		err = <-served
	}
	// Admission is closed under the same lock used before requests.Add. No HTTP
	// handler or login survives the server's foreground lifetime or App.Close.
	s.requests.Wait()
	s.mu.Lock()
	cancel, authDone := s.authCancel, s.authDone
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if authDone != nil {
		<-authDone
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func jsonResult(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int) { http.Error(w, http.StatusText(status), status) }
func (s *Server) authorized(r *http.Request) bool {
	token, e := s.Secrets.Get(r.Context(), s.Config.TokenRef)
	if e != nil || len(token) < 24 {
		return false
	}
	defer func() {
		for i := range token {
			token[i] = 0
		}
	}()
	if value, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && subtle.ConstantTimeCompare([]byte(value), token) == 1 {
		return true
	}
	cookie, e := r.Cookie("collie_session")
	if e != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Now().Before(s.sessionExpires) && s.session != "" && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.session)) == 1
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		fail(w, 503)
		return
	}
	s.requests.Add(1)
	s.mu.Unlock()
	defer s.requests.Done()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; font-src 'self'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	if r.URL.Path == "/feishu/callback" && s.Callback != nil {
		s.Callback.ServeHTTP(w, r)
		return
	}
	// Reject DNS rebinding, cross-site POSTs and wildcard Origin. No CORS API.
	host, _, e := net.SplitHostPort(r.Host)
	if e != nil || (host != "127.0.0.1" && host != "::1") {
		fail(w, 403)
		return
	}
	if r.Method != "GET" && r.Method != "POST" {
		fail(w, 405)
		return
	}
	if r.Method == "POST" {
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			fail(w, 403)
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			fail(w, 403)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	}
	if file, ok := map[string]struct{ path, contentType string }{
		"/style.css":                    {"assets/style.css", "text/css"},
		"/app.js":                       {"assets/client.js", "text/javascript"},
		"/licenses.txt":                 {"assets/licenses.txt", "text/plain; charset=utf-8"},
		"/brand/collie.png":             {"assets/brand/collie.png", "image/png"},
		"/brand/bricolagegrotesque.ttf": {"assets/brand/bricolagegrotesque.ttf", "font/ttf"},
		"/brand/dmsans.ttf":             {"assets/brand/dmsans.ttf", "font/ttf"},
		"/favicon.png":                  {"assets/brand/collie.png", "image/png"},
	}[r.URL.Path]; ok {
		body, err := assets.ReadFile(file.path)
		if err != nil {
			fail(w, 500)
			return
		}
		w.Header().Set("Content-Type", file.contentType)
		_, _ = w.Write(body)
		return
	}
	if r.URL.Path == "/session" && r.Method == "POST" {
		if r.ParseForm() != nil {
			fail(w, 400)
			return
		}
		token, e := s.Secrets.Get(r.Context(), s.Config.TokenRef)
		if e != nil || len(token) < 24 || subtle.ConstantTimeCompare(token, []byte(r.FormValue("token"))) != 1 {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(401)
			s.render(w, page{Login: true, Error: "That workspace token was not accepted.", Next: safePageReturn(r.FormValue("next"))})
			return
		}
		for i := range token {
			token[i] = 0
		}
		bytes := make([]byte, 32)
		if _, e = rand.Read(bytes); e != nil {
			fail(w, 500)
			return
		}
		session := base64.RawURLEncoding.EncodeToString(bytes)
		s.mu.Lock()
		s.session = session
		s.sessionExpires = time.Now().Add(time.Hour)
		s.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "collie_session", Value: session, HttpOnly: true, SameSite: http.SameSiteStrictMode, Path: "/", MaxAge: 3600})
		http.Redirect(w, r, safePageReturn(r.FormValue("next")), 303)
		return
	}
	if !s.authorized(r) {
		_, view, _ := pageRoute(r.URL.Path)
		if view != "" && r.Method == "GET" {
			if r.URL.Path != "/" {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(401)
			}
			s.render(w, page{Login: true, Next: safePageReturn(r.URL.RequestURI())})
			return
		}
		fail(w, 401)
		return
	}
	if s.Core.CheckChannel(r.Context(), "web", Subject, Destination) != nil {
		fail(w, 403)
		return
	}
	if r.Method == "GET" {
		s.get(w, r)
		return
	}
	if e := s.post(w, r); e != nil {
		s.postFailure(w, r, e)
		return
	}
}

// AuthURL restricts clickable authentication links to the existing provider's
// secure browser flow. Events remain ephemeral, authenticated, and value-free in logs.
func AuthURL(value string) string {
	u, e := url.Parse(value)
	if e != nil || u.Scheme != "https" || u.User != nil {
		return ""
	}
	return value
}
