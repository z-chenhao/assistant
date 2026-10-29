package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"

	"github.com/z-chenhao/erii/internal/delivery/ui"
	"github.com/z-chenhao/erii/internal/interaction"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/task"
)

type shownMessage struct {
	Role       string
	Sequence   int64
	Projection shownProjection
}
type shownProjection struct {
	Key      string
	HTML     template.HTML
	Spec     string
	Fallback string
}
type page struct {
	Trace      *traceView
	Context    *contextView
	Accounts   []llm.AuthConnection
	Bindings   []interaction.Binding
	Preference interaction.Preference
	Login      bool
	Tab        string
	Status     Status
	Cursor     int64
	Latest     int64
	Before     int64
	Messages   []shownMessage
	Activity   []task.Summary
	Approvals  []shownProjection
	Profiles   []llm.Profile
	SourceID   string
	Auth       llm.AuthState
	AccountID  string
}

func (s *Server) show(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := page{Tab: r.URL.Path}
	var err error
	p.Status, err = s.Core.WebStatus(ctx)
	if err != nil {
		fail(w, 503)
		return
	}
	if p.Tab == "/settings" {
		if s.Accounts != nil {
			p.Accounts, err = s.Accounts.Store.Accounts(ctx)
			if err != nil {
				fail(w, 503)
				return
			}
		}
		p.Bindings, err = s.Core.ChannelBindings(ctx)
		if err != nil {
			fail(w, 503)
			return
		}
		p.Preference, err = s.Core.ChannelPreference(ctx)
		if err != nil {
			fail(w, 503)
			return
		}
		p.Profiles, err = s.Models.ListProfiles(ctx)
		if err != nil {
			fail(w, 503)
			return
		}
	}
	if p.Tab == "/progress" {
		p.Activity, err = s.Core.Activity(ctx)
		if err != nil {
			fail(w, 503)
			return
		}
	}
	if p.Tab == "/developer/context" {
		p.Context, err = contextQuery(r)
		if err == nil {
			err = s.loadContext(ctx, p.Context, "")
		}
		if err != nil {
			inspectionFailure(w, err)
			return
		}
	}
	if p.Tab == "/developer/observability" {
		p.Trace, err = traceQuery(r)
		if err == nil {
			err = s.loadTrace(ctx, p.Trace)
		}
		if err != nil {
			inspectionFailure(w, err)
			return
		}
	}

	if p.Tab == "/" {
		p.Cursor, err = conversationCursor(r)
		if err != nil {
			fail(w, 400)
			return
		}
		if err = s.loadConversation(ctx, &p); err != nil {
			fail(w, 503)
			return
		}
	}

	key := make([]byte, 16)
	if _, err = rand.Read(key); err != nil {
		fail(w, 500)
		return
	}
	p.SourceID = hex.EncodeToString(key)
	s.mu.Lock()
	p.Auth = s.auth
	p.AccountID = s.accountID
	s.mu.Unlock()
	s.render(w, p)
}
func (s *Server) render(w http.ResponseWriter, p page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := layout.Execute(w, p); err != nil {
		return
	}
}

func conversationCursor(r *http.Request) (int64, error) {
	value := r.URL.Query().Get("before")
	if value == "" {
		return 0, nil
	}
	before, err := strconv.ParseInt(value, 10, 64)
	if err != nil || before <= 0 {
		return 0, strconv.ErrSyntax
	}
	return before, nil
}

func (s *Server) loadConversation(ctx context.Context, p *page) error {
	approvals, err := s.Core.Approvals(ctx)
	if err != nil {
		return err
	}
	for _, a := range approvals {
		if a.Status == "pending" {
			spec := ui.Spec{SchemaVersion: ui.Version, Fallback: a.Arguments, Components: []ui.Component{{Type: "confirmation", Text: a.Tool + "\n" + a.Arguments, ApprovalID: a.ID, Hash: a.Hash}}}
			raw, _ := json.Marshal(spec)
			p.Approvals = append(p.Approvals, shownProjection{Key: a.ID, HTML: ui.HTML(raw, spec.Fallback), Spec: string(raw), Fallback: spec.Fallback})
		}
	}
	messages, err := s.Core.Conversation(ctx, p.Cursor)
	p.Before = messages.Before
	if err != nil {
		return err
	}
	for _, m := range messages.Messages {
		fallback := ""
		for _, c := range m.Content {
			if c.Type == "text" {
				fallback += c.Text + "\n"
			} else if c.ArtifactRef != "" {
				fallback += c.Type + ": " + c.ArtifactRef + "\n"
			}
		}
		raw, err := s.Core.WebProjection(ctx, m)
		if err != nil {
			return err
		}
		p.Messages = append(p.Messages, shownMessage{Role: m.Role, Sequence: m.Sequence, Projection: shownProjection{HTML: ui.HTML(raw, fallback), Spec: string(raw), Fallback: fallback}})
	}
	for _, m := range p.Messages {
		p.Latest = max(p.Latest, m.Sequence)
	}
	if p.Cursor != 0 {
		latest, err := s.Core.Conversation(ctx, 0)
		if err != nil {
			return err
		}
		for _, m := range latest.Messages {
			p.Latest = max(p.Latest, m.Sequence)
		}
	}
	return nil
}
