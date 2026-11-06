package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/conversation"
	"github.com/z-chenhao/assistant/internal/delivery/ui"
	"github.com/z-chenhao/assistant/internal/interaction"
)

type uiInteractions interface {
	WebUIState(context.Context, string) (ui.ViewState, error)
	SaveWebUIState(context.Context, string, ui.ViewState) (ui.ViewState, error)
	RequestWebUIAction(context.Context, string, string) (agent.Receipt, error)
	WebUIActionResult(context.Context, string, string) (ui.ActionResult, error)
}

func interactionError(w http.ResponseWriter, err error) {
	code, kind := 503, "unavailable"
	switch {
	case errors.Is(err, conversation.ErrInvalid):
		code, kind = 400, "invalid"
	case errors.Is(err, conversation.ErrConflict), errors.Is(err, agent.ErrConflict):
		code, kind = 409, "changed"
	case errors.Is(err, agent.ErrNotFound):
		code, kind = 404, "missing"
	case errors.Is(err, interaction.ErrUnauthorized):
		code, kind = 403, "access_changed"
	}
	w.WriteHeader(code)
	jsonResult(w, map[string]string{"error": kind})
}
func (s *Server) uiInteraction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	core, ok := s.Core.(uiInteractions)
	if !ok {
		fail(w, 503)
		return
	}
	if r.Method == "GET" {
		var value any
		var err error
		if r.URL.Path == "/api/ui/state" {
			value, err = core.WebUIState(r.Context(), r.URL.Query().Get("ref"))
		} else {
			value, err = core.WebUIActionResult(r.Context(), r.URL.Query().Get("ref"), r.URL.Query().Get("source"))
		}
		if err != nil {
			interactionError(w, err)
			return
		}
		jsonResult(w, value)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 24<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input struct {
		Ref     string          `json:"ref"`
		Source  string          `json:"source_id,omitempty"`
		Version int64           `json:"version,omitempty"`
		State   json.RawMessage `json:"state,omitempty"`
		Context json.RawMessage `json:"model_context,omitempty"`
	}
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		interactionError(w, conversation.ErrInvalid)
		return
	}
	if r.URL.Path == "/api/ui/state" {
		if input.Source != "" {
			interactionError(w, conversation.ErrInvalid)
			return
		}
		state, err := core.SaveWebUIState(r.Context(), input.Ref, ui.ViewState{Version: input.Version, State: input.State, ModelContext: input.Context})
		if err != nil {
			interactionError(w, err)
			return
		}
		jsonResult(w, state)
	} else {
		if len(input.State) > 0 || len(input.Context) > 0 || input.Version != 0 {
			interactionError(w, conversation.ErrInvalid)
			return
		}
		receipt, err := core.RequestWebUIAction(r.Context(), input.Ref, input.Source)
		if err != nil {
			interactionError(w, err)
			return
		}
		if s.Wake != nil {
			s.Wake()
		}
		jsonResult(w, map[string]any{"accepted": true, "message_id": receipt.MessageID})
	}
}
func (s *Server) uiImage(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	d, err := s.Core.DescribeArtifact(r.Context(), ref)
	if err != nil {
		fail(w, 404)
		return
	}
	switch d.MediaType {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
	default:
		fail(w, 415)
		return
	}
	src, err := s.Core.OpenArtifact(r.Context(), ref)
	if err != nil {
		fail(w, 404)
		return
	}
	defer src.Close()
	w.Header().Set("Content-Type", d.MediaType)
	_, _ = io.Copy(w, src)
}
