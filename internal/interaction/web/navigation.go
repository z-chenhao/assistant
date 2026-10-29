package web

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/z-chenhao/erii/internal/observability"
	"github.com/z-chenhao/erii/internal/task"
)

func pageRoute(path string) (tab, view, title string) {
	switch {
	case path == "/":
		return "/", "conversation", "Conversation"
	case path == "/tasks" || path == "/progress":
		return "/tasks", "tasks", "Tasks"
	case strings.HasPrefix(path, "/tasks/") && task.ValidRef(strings.TrimPrefix(path, "/tasks/")):
		return "/tasks", "task", "Task"
	case path == "/settings/notifications":
		return "/settings", "settings", "Notifications"
	case path == "/settings" || path == "/settings/models":
		return "/settings", "settings", "Models"
	case path == "/settings/models/new":
		return "/settings", "settings", "Connect a model"
	case strings.HasPrefix(path, "/settings/models/") && pageLeaf(strings.TrimPrefix(path, "/settings/models/")):
		return "/settings", "settings", "Model details"
	case path == "/developer/observability":
		return "/developer/observability", "traces", "Traces"
	case strings.HasPrefix(path, "/developer/observability/") && pageLeaf(strings.TrimPrefix(path, "/developer/observability/")) || path == "/developer/context":
		return "/developer/observability", "trace", "Trace details"
	}
	return "", "", ""
}
func safePageReturn(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") || strings.ContainsAny(raw, "\r\n\\") {
		return "/"
	}
	if _, view, _ := pageRoute(u.Path); view == "" {
		return "/"
	}
	return u.String()
}
func (s *Server) pageError(w http.ResponseWriter, r *http.Request, err error) {
	code := 503
	message := "This page is temporarily unavailable. Your work has not been changed."
	if errors.Is(err, errPageNotFound) || errors.Is(err, task.ErrNotFound) || errors.Is(err, observability.ErrNotFound) {
		code = 404
		message = "This item could not be found in your workspace."
	}
	if errors.Is(err, task.ErrInvalid) || errors.Is(err, observability.ErrInvalid) {
		code = 400
		message = "This page link is invalid. Return to the list and choose an item."
	}
	tab, _, title := pageRoute(r.URL.Path)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	s.render(w, page{Tab: tab, View: "error", Title: title, Path: r.URL.Path, Error: message})
}

// Old public bookmarks redirect to the single canonical view; no parallel UI.
func legacyPageRedirect(w http.ResponseWriter, r *http.Request) bool {
	target := ""
	switch r.URL.Path {
	case "/settings":
		target = "/settings/models"
	case "/progress":
		target = "/tasks"
	case "/developer/context":
		if _, err := contextQuery(r); err != nil {
			return false
		}
		q := r.URL.Query()
		run := q.Get("run")
		q.Del("run")
		q.Set("tab", "context")
		q.Set("ctx", "1")
		target = tracePath(run) + "?" + q.Encode()
	case "/developer/observability":
		if _, err := traceQuery(r); err != nil {
			return false
		}
		if run := r.URL.Query().Get("run"); run != "" {
			q := r.URL.Query()
			q.Del("run")
			target = tracePath(run) + "?" + q.Encode()
		}
	}
	if target == "" {
		return false
	}
	http.Redirect(w, r, target, 303)
	return true
}

func pageLeaf(value string) bool {
	return value != "" && len(value) <= 256 && !strings.ContainsAny(value, "/\\\r\n\x00")
}
