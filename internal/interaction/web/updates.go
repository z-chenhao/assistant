package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
)

// updates is a view of committed product data, not an event log or a second
// execution channel. Authentication and binding revocation run before every
// request, including conditional requests. No composer/form identity is returned.
func (s *Server) updates(w http.ResponseWriter, r *http.Request) {
	p := page{Tab: r.URL.Query().Get("view")}
	fragments := map[string]string{"workspace-status": "status"}
	switch p.Tab {
	case "/":
		fragments["conversation-messages"], fragments["conversation-approvals"] = "conversation", "approvals"
	case "/progress":
		fragments["ongoing-work"] = "progress"
	case "/settings":
		fragments["signin-status"] = "signin"
	default:
		fail(w, 400)
		return
	}
	var err error
	if p.Cursor, err = conversationCursor(r); err != nil || p.Cursor != 0 && p.Tab != "/" {
		fail(w, 400)
		return
	}
	if p.Status, err = s.Core.WebStatus(r.Context()); err != nil {
		fail(w, 503)
		return
	}
	switch p.Tab {
	case "/":
		err = s.loadConversation(r.Context(), &p)
	case "/progress":
		p.Activity, err = s.Core.Activity(r.Context())
	case "/settings":
		s.mu.Lock()
		p.Auth = s.auth
		s.mu.Unlock()
	}
	if err != nil {
		fail(w, 503)
		return
	}
	regions := map[string]string{}
	for id, name := range fragments {
		var html bytes.Buffer
		if err = layout.ExecuteTemplate(&html, name, p); err != nil {
			fail(w, 500)
			return
		}
		regions[id] = html.String()
	}
	body, err := json.Marshal(struct {
		View    string            `json:"view"`
		Before  int64             `json:"before"`
		Latest  int64             `json:"latest"`
		Regions map[string]string `json:"regions"`
	}{p.Tab, p.Cursor, p.Latest, regions})
	if err != nil {
		fail(w, 500)
		return
	}
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(body))
	w.Header().Set("ETag", etag)
	w.Header().Set("Vary", "Cookie, Authorization")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}
