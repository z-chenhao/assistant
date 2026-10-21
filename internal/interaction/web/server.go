// Package web presents the authenticated local control and conversation surface.
// HTTP handlers do not implement a second execution or approval lifecycle.
package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/z-chenhao/erii/internal/delivery/ui"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/artifact"
	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/interaction"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/llm/account"
	"github.com/z-chenhao/erii/internal/task"
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
	Readiness     string `json:"readiness"`
	Status        string `json:"status"`
	ConfigVersion int64  `json:"config_version"`
}
type Core interface {
	ChannelBindings(context.Context) ([]interaction.Binding, error)
	ChannelPreference(context.Context) (interaction.Preference, error)
	PreferChannel(context.Context, string, string, int64) error
	WebStatus(context.Context) (Status, error)
	History(context.Context) ([]conversation.Message, error)
	Activity(context.Context) ([]task.Summary, error)
	Approvals(context.Context) ([]interaction.Approval, error)
	AcceptInbound(context.Context, interaction.InboundEvent) (agent.Receipt, error)
	DecideInbound(context.Context, interaction.InboundEvent, string, string, string) error
	CheckChannel(context.Context, string, string, string) error
	ImportChannelArtifact(context.Context, interaction.InboundEvent, string, artifact.Input, io.Reader) (string, error)
	SaveArtifact(context.Context, artifact.Input, io.Reader) (artifact.Descriptor, error)
	OpenArtifact(context.Context, string) (io.ReadCloser, error)
	DescribeArtifact(context.Context, string) (artifact.Descriptor, error)
	SaveWebProjection(context.Context, conversation.Message) (json.RawMessage, error)
}
type Models interface {
	ListProfiles(context.Context) ([]llm.Profile, error)
	AddProfile(context.Context, llm.ProfileInput) (llm.Profile, error)
}
type Server struct {
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
	cookie, e := r.Cookie("erii_session")
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
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
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
	if r.URL.Path == "/style.css" {
		w.Header().Set("Content-Type", "text/css")
		_, _ = io.WriteString(w, style)
		return
	}
	if r.URL.Path == "/app.js" {
		w.Header().Set("Content-Type", "text/javascript")
		_, _ = io.WriteString(w, script)
		return
	}
	if r.URL.Path == "/session" && r.Method == "POST" {
		if r.ParseForm() != nil {
			fail(w, 400)
			return
		}
		token, e := s.Secrets.Get(r.Context(), s.Config.TokenRef)
		if e != nil || len(token) < 24 || subtle.ConstantTimeCompare(token, []byte(r.FormValue("token"))) != 1 {
			fail(w, 401)
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
		http.SetCookie(w, &http.Cookie{Name: "erii_session", Value: session, HttpOnly: true, SameSite: http.SameSiteStrictMode, Path: "/", MaxAge: 3600})
		http.Redirect(w, r, "/", 303)
		return
	}
	if !s.authorized(r) {
		if r.URL.Path == "/" && r.Method == "GET" {
			s.render(w, page{Login: true})
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
		fail(w, 409)
		return
	}
}
func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/conversation":
		m, e := s.Core.History(r.Context())
		if e != nil {
			fail(w, 503)
			return
		}
		jsonResult(w, m)
	case "/api/progress":
		a, e := s.Core.Activity(r.Context())
		if e != nil {
			fail(w, 503)
			return
		}
		jsonResult(w, a)
	case "/api/status":
		a, e := s.Core.WebStatus(r.Context())
		if e != nil {
			fail(w, 503)
			return
		}
		jsonResult(w, a)
	case "/api/auth":
		s.mu.Lock()
		defer s.mu.Unlock()
		jsonResult(w, struct {
			llm.AuthState
			AccountID string `json:"account_id,omitempty"`
		}{s.auth, s.accountID})
	case "/artifact":
		ref := r.URL.Query().Get("ref")
		d, e := s.Core.DescribeArtifact(r.Context(), ref)
		if e != nil {
			fail(w, 404)
			return
		}
		src, e := s.Core.OpenArtifact(r.Context(), ref)
		if e != nil {
			fail(w, 404)
			return
		}
		defer src.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": d.Name}))
		_, _ = io.Copy(w, src)
	case "/", "/settings", "/progress":
		s.show(w, r)
	default:
		fail(w, 404)
	}
}
func (s *Server) post(w http.ResponseWriter, r *http.Request) error {
	if r.URL.Path == "/api/message" {
		var input struct {
			SourceID string                     `json:"source_id"`
			Content  []conversation.ContentPart `json:"content"`
		}
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		if d.Decode(&input) != nil {
			fail(w, 400)
			return nil
		}
		receipt, e := s.Core.AcceptInbound(r.Context(), interaction.InboundEvent{Channel: "web", Subject: Subject, Destination: Destination, EventID: input.SourceID, SourceID: input.SourceID, Content: input.Content})
		if e != nil {
			return e
		}
		if s.Wake != nil {
			s.Wake()
		}
		jsonResult(w, map[string]any{"accepted": true, "message_id": receipt.MessageID})
		return nil
	}
	if r.URL.Path == "/message" {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			return err
		}
		defer r.MultipartForm.RemoveAll()
		key := r.FormValue("source_id")
		e := interaction.InboundEvent{Channel: "web", Subject: Subject, Destination: Destination, EventID: key, SourceID: key, Content: conversation.Text(r.FormValue("text"))}
		if err := e.Validate(); err != nil {
			return err
		}
		file, header, err := r.FormFile("file")
		if err == nil {
			defer file.Close()
			data, err := io.ReadAll(io.LimitReader(file, (30<<20)+1))
			if err != nil || len(data) > 30<<20 {
				return artifact.ErrLimit
			}
			input := artifact.Input{Name: header.Filename, MediaType: header.Header.Get("Content-Type")}
			ref, err := s.Core.ImportChannelArtifact(r.Context(), e, ui.Hash([]any{input.Name, input.MediaType, ui.Hash(data)}), input, bytes.NewReader(data))
			if err != nil {
				return err
			}
			e.Content = append(e.Content, conversation.ContentPart{Type: artifact.ContentType(input.MediaType), ArtifactRef: ref})
		} else if err != http.ErrMissingFile {
			return err
		}
		if _, err = s.Core.AcceptInbound(r.Context(), e); err != nil {
			return err
		}
		if s.Wake != nil {
			s.Wake()
		}
		http.Redirect(w, r, "/", 303)
		return nil
	}
	if r.URL.Path == "/upload" {
		if e := r.ParseMultipartForm(1 << 20); e != nil {
			return e
		}
		defer r.MultipartForm.RemoveAll()
		file, h, e := r.FormFile("file")
		if e != nil {
			return e
		}
		defer file.Close()
		d, e := s.Core.SaveArtifact(r.Context(), artifact.Input{Name: h.Filename, MediaType: h.Header.Get("Content-Type")}, file)
		if e != nil {
			return e
		}
		jsonResult(w, d)
		return nil
	}
	if e := r.ParseForm(); e != nil {
		return e
	}
	switch r.URL.Path {
	case "/approval":
		key := r.FormValue("source_id")
		if key == "" {
			key = "approval:" + r.FormValue("approval_id") + ":" + r.FormValue("decision")
		}
		text := r.FormValue("approval_id") + ":" + r.FormValue("hash") + ":" + r.FormValue("decision")
		event := interaction.InboundEvent{Channel: "web", Subject: Subject, Destination: Destination, EventID: key, SourceID: key, Content: conversation.Text(text)}
		if e := s.Core.DecideInbound(r.Context(), event, r.FormValue("approval_id"), r.FormValue("hash"), r.FormValue("decision")); e != nil {
			return e
		}
		if s.Wake != nil {
			s.Wake()
		}
	case "/model/add":
		input := llm.ProfileInput{Name: r.FormValue("model"), Endpoint: r.FormValue("endpoint"), ModelID: r.FormValue("model"), ProviderKind: "openai_compatible", WireAPI: "openai_completions", ContextWindow: 8192, MaxOutputTokens: 1024, Compatibility: llm.Compatibility{ToolResultFormat: "openai"}, Capabilities: llm.Capabilities{TextInput: true}}
		if r.FormValue("account") != "" {
			input.AuthConnectionID = r.FormValue("account")
			input.ProviderKind = "openai_account"
			input.WireAPI = "openai_responses"
			input.Endpoint = ""
			if s.AccountCatalog == nil {
				return llm.ErrNoBridge
			}
			catalog, err := s.AccountCatalog(r.Context(), input.AuthConnectionID)
			if err != nil {
				return err
			}
			found := false
			for _, m := range catalog {
				if m.ModelID == input.ModelID {
					input.ContextWindow = m.ContextWindow
					input.MaxOutputTokens = min(m.MaxOutputTokens, m.ContextWindow)
					found = true
				}
			}
			if !found {
				return llm.ModelIncompatible
			}
		}
		if _, e := s.Models.AddProfile(r.Context(), input); e != nil {
			return e
		}
	case "/model/test":
		if _, e := s.Control.Test(r.Context(), r.FormValue("profile")); e != nil {
			return e
		}
	case "/model/select":
		if r.FormValue("consent") != "yes" {
			return llm.ErrConflict
		}
		pv, e := strconv.ParseInt(r.FormValue("profile_version"), 10, 64)
		if e != nil {
			return e
		}
		cv, e := strconv.ParseInt(r.FormValue("config_version"), 10, 64)
		if e != nil {
			return e
		}
		if e = s.Control.Select(r.Context(), r.FormValue("profile"), pv, cv); e != nil {
			return e
		}
	case "/notification":
		version, e := strconv.ParseInt(r.FormValue("version"), 10, 64)
		if e != nil {
			return e
		}
		channel, binding, ok := strings.Cut(r.FormValue("target"), ":")
		if !ok {
			return interaction.ErrUnauthorized
		}
		if e = s.Core.PreferChannel(r.Context(), channel, binding, version); e != nil {
			return e
		}
	case "/auth/login":
		s.mu.Lock()
		if s.auth.Status == "pending" {
			s.mu.Unlock()
			return llm.ErrConflict
		}
		lifetime := s.lifetime
		if lifetime == nil {
			lifetime = context.Background()
		}
		ctx, cancel := context.WithCancel(lifetime)
		s.authCancel = cancel
		s.auth = llm.AuthState{Status: "pending", Events: []llm.AuthEvent{}}
		s.accountID = ""
		done := make(chan struct{})
		s.authDone = done
		s.mu.Unlock()
		accountID := r.FormValue("account")
		go func() {
			defer close(done)
			a, e := s.Accounts.Login(ctx, accountID, func(event llm.AuthEvent) {
				s.mu.Lock()
				defer s.mu.Unlock()
				s.auth.Events = append(s.auth.Events, event)
			})
			s.mu.Lock()
			defer s.mu.Unlock()
			s.auth.Status = "connected"
			if e != nil {
				s.auth.Status = "reauth_required"
			}
			s.accountID = a.ConnectionID
		}()
	case "/auth/logout":
		if _, err := s.Accounts.Logout(r.Context(), r.FormValue("account")); err != nil {
			return err
		}
	case "/auth/cancel":
		s.mu.Lock()
		cancel := s.authCancel
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	default:
		fail(w, 404)
		return nil
	}
	http.Redirect(w, r, "/", 303)
	return nil
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
