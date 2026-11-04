package web

import (
	"context"
	"net/http"
	"strings"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/conversation"
)

type RichForms interface {
	RespondWebForm(context.Context, string, string, map[string]string) (agent.Receipt, error)
}

func (s *Server) respondUIForm(w http.ResponseWriter, r *http.Request) error {
	forms, ok := s.Core.(RichForms)
	if !ok {
		return conversation.ErrConflict
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	if err := r.ParseForm(); err != nil {
		return conversation.ErrInvalid
	}
	values := map[string]string{}
	for key, items := range r.PostForm {
		if len(items) != 1 {
			return conversation.ErrInvalid
		}
		if name, ok := strings.CutPrefix(key, "field."); ok {
			values[name] = items[0]
		} else if key != "message_id" && key != "form_id" {
			return conversation.ErrInvalid
		}
	}
	if len(values) > 16 {
		return conversation.ErrInvalid
	}
	if id := r.PostForm.Get("message_id"); id == "" || len(id) > 256 || len(r.PostForm.Get("form_id")) != 64 {
		return conversation.ErrInvalid
	}
	receipt, err := forms.RespondWebForm(r.Context(), r.PostForm.Get("message_id"), r.PostForm.Get("form_id"), values)
	if err != nil {
		return err
	}
	if s.Wake != nil {
		s.Wake()
	}
	if r.Header.Get("Accept") == "application/json" {
		jsonResult(w, map[string]any{"accepted": true, "message_id": receipt.MessageID})
	} else {
		http.Redirect(w, r, "/?notice=feedback-saved", http.StatusSeeOther)
	}
	return nil
}
