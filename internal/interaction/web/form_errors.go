package web

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/z-chenhao/erii/internal/llm"
)

func (s *Server) postFailure(w http.ResponseWriter, r *http.Request, cause error) {
	if r.Header.Get("Accept") == "application/json" {
		fail(w, 409)
		return
	}
	path := "/"
	query := url.Values{}
	switch r.URL.Path {
	case "/approval":
		u, _ := url.Parse(safePageReturn(r.FormValue("return_to")))
		path, query = u.Path, u.Query()
	case "/message":
		query.Set("task", r.FormValue("task_ref"))
	case "/model/add":
		path = "/settings/models/new"
		query.Set("provider", "compatible")
		if r.FormValue("account") != "" {
			query.Set("provider", "chatgpt")
			query.Set("account", r.FormValue("account"))
		}
	case "/model/test", "/model/select":
		path = modelURL(r.FormValue("profile"))
	case "/notification":
		path = "/settings/notifications"
	case "/auth/login", "/auth/logout", "/auth/cancel":
		path = "/settings/models/new"
		query.Set("provider", "chatgpt")
	}
	next := r.Clone(r.Context())
	u := *r.URL
	u.Path = path
	u.RawQuery = query.Encode()
	next.URL = &u
	p, err := s.page(r.Context(), next)
	if err != nil {
		s.pageError(w, next, err)
		return
	}
	p.Error = "This action could not be completed. Review the current state before trying again."
	if errors.Is(cause, llm.AuthRequired) {
		p.Error = "This provider needs authentication. Reconnect the account and verify the model."
	}
	if errors.Is(cause, llm.ModelIncompatible) {
		p.Error = "The model did not pass compatibility checks. Check the selected model and its declared capabilities."
	}
	if errors.Is(cause, llm.ErrConflict) {
		p.Error = "These settings changed. Review the current values before confirming again."
	}
	if r.URL.Path == "/message" {
		p.Draft = r.FormValue("text")
		p.SourceID = r.FormValue("source_id")
	}
	if p.Settings != nil && p.Settings.Section == "new" {
		p.Settings.Model = r.FormValue("model")
		p.Settings.Name = r.FormValue("name")
		p.Settings.ImageInput = r.FormValue("image_input") == "yes"
		if endpoint, err := url.Parse(r.FormValue("endpoint")); err == nil && endpoint.User == nil && endpoint.RawQuery == "" && endpoint.Fragment == "" {
			p.Settings.Endpoint = endpoint.String()
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(409)
	s.render(w, p)
}
