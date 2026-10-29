package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"net/http"
	"strconv"

	"github.com/z-chenhao/collie/internal/interaction"
	"github.com/z-chenhao/collie/internal/llm"
)

type shownMessage struct {
	Role       string
	Sequence   int64
	Projection shownProjection
	Approval   *approvalView
}
type shownProjection struct {
	Key            string
	HTML           template.HTML
	Spec, Fallback string
}
type page struct {
	Capabilities                                      []Capability
	Capability                                        *Capability
	Trace                                             *traceView
	Tasks                                             *taskView
	Settings                                          *settingsView
	Accounts                                          []llm.AuthConnection
	Bindings                                          []interaction.Binding
	Preference                                        interaction.Preference
	Login                                             bool
	Tab, View, Title, Path, Next, Error, Notice, Live string
	Status                                            Status
	Cursor, Latest, Before                            int64
	Messages                                          []shownMessage
	Approvals                                         []approvalView
	Profiles                                          []llm.Profile
	SourceID                                          string
	Auth                                              llm.AuthState
	AccountID                                         string
	Draft                                             string
	DraftTask                                         *taskContext
}
type taskContext struct{ Ref, Title string }

func sourceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
func (s *Server) page(ctx context.Context, r *http.Request) (page, error) {
	tab, view, title := pageRoute(r.URL.Path)
	p := page{Tab: tab, View: view, Title: title, Path: r.URL.Path, SourceID: sourceID(), Notice: notice(r.URL.Query().Get("notice"))}
	if view == "" {
		return p, errPageNotFound
	}
	if view == "conversation" {
		p.Live = "/"
	}
	if view == "tasks" || view == "task" || view == "settings" {
		p.Live = r.URL.Path
	}
	var err error
	p.Status, err = s.Core.WebStatus(ctx)
	if err != nil {
		return p, err
	}
	s.mu.Lock()
	p.Auth = s.auth
	p.AccountID = s.accountID
	s.mu.Unlock()
	switch p.View {
	case "capabilities":
		err = s.loadCapabilities(ctx, r, &p)
	case "settings":
		err = s.loadSettings(ctx, r, &p)
	case "tasks", "task":
		err = s.loadTasks(ctx, r, &p)
	case "traces", "trace":
		p.Trace, err = tracePageQuery(r)
		if err == nil {
			err = s.loadTrace(ctx, p.Trace)
		}
		if p.Trace != nil && p.Trace.Run != "" {
			p.View = "trace"
		}
	case "conversation":
		p.Cursor, err = conversationCursor(r)
		if err != nil {
			return p, err
		}
		if ref := r.URL.Query().Get("task"); ref != "" {
			if s.Tasks == nil {
				return p, errPageNotFound
			}
			d, e := s.Tasks.Detail(ctx, ref, "")
			if e != nil {
				return p, e
			}
			p.DraftTask = &taskContext{Ref: d.TaskRef, Title: d.Title}
		}
		err = s.loadConversation(ctx, &p)
	}
	return p, err
}
func (s *Server) show(w http.ResponseWriter, r *http.Request) {
	p, err := s.page(r.Context(), r)
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	s.render(w, p)
}
func (s *Server) render(w http.ResponseWriter, p page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = layout.Execute(w, p)
}
func conversationCursor(r *http.Request) (int64, error) {
	raw := r.URL.Query().Get("before")
	if raw == "" {
		return 0, nil
	}
	before, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || before <= 0 {
		return 0, strconv.ErrSyntax
	}
	return before, nil
}
func notice(code string) string {
	switch code {
	case "added":
		return "Model saved. Verify the connection before using it."
	case "tested":
		return "Connection verified. You can now use this model for new work."
	case "active":
		return "Model selected for new work."
	case "saved":
		return "Notification preference saved."
	case "signed-out":
		return "Provider account signed out."
	}
	return ""
}
