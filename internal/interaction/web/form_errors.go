package web

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/conversation"
	"github.com/z-chenhao/assistant/internal/interaction"
	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/task"
)

func (s *Server) postFailure(w http.ResponseWriter, r *http.Request, cause error) {
	if r.Header.Get("Accept") == "application/json" {
		if r.URL.Path == "/task/feedback" {
			code, kind := 503, "unconfirmed"
			switch {
			case errors.Is(cause, task.ErrInvalid), errors.Is(cause, conversation.ErrInvalid):
				code, kind = 400, "invalid_answer"
			case errors.Is(cause, task.ErrFeedbackStale), errors.Is(cause, conversation.ErrConflict), errors.Is(cause, agent.ErrConflict):
				code, kind = 409, "feedback_changed"
			case errors.Is(cause, interaction.ErrUnauthorized):
				code, kind = 403, "access_changed"
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			jsonResult(w, struct {
				Error string `json:"error"`
			}{kind})
			return
		}
		fail(w, 409)
		return
	}
	path := "/"
	query := url.Values{}
	switch r.URL.Path {
	case "/approval", "/task/feedback":
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
	if r.URL.Path == "/task/feedback" {
		p.Error = "This question or review may have changed. Check the current task before answering again."
		p.FeedbackDraft = r.FormValue("answer")
		if errors.Is(cause, task.ErrInvalid) || errors.Is(cause, conversation.ErrInvalid) {
			p.Error = "This answer is too long or invalid. Edit it before sending again."
		}
		if p.Attention != nil {
			for i := range p.Attention.Feedbacks {
				if p.Attention.Feedbacks[i].Ref == r.FormValue("feedback_ref") {
					p.Attention.Feedbacks[i].Draft = p.FeedbackDraft
					p.FeedbackDraft = ""
				}
			}
		}
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
	status := 409
	if r.URL.Path == "/task/feedback" && (errors.Is(cause, task.ErrInvalid) || errors.Is(cause, conversation.ErrInvalid)) {
		status = 400
	}
	w.WriteHeader(status)
	s.render(w, r, p)
}
