package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/conversation"
	"github.com/z-chenhao/assistant/internal/interaction"
	"github.com/z-chenhao/assistant/internal/task"
)

const taskQuestionSubmit = "steward_task_question_submit"

func taskQuestionCard(ref, text string) map[string]any {
	plain := func(text string) map[string]string { return map[string]string{"tag": "plain_text", "content": text} }
	return map[string]any{"schema": "2.0", "body": map[string]any{"elements": []any{
		map[string]any{"tag": "div", "text": plain(text)},
		map[string]any{"tag": "form", "name": "steward_task_question", "elements": []any{
			map[string]any{"tag": "input", "name": "answer", "required": true, "placeholder": plain("Your answer")},
			map[string]any{"tag": "button", "name": taskQuestionSubmit, "type": "primary", "text": plain("Send answer"), "form_action_type": "submit",
				"behaviors": []any{map[string]any{"type": "callback", "value": map[string]string{"feedback_ref": ref}}}},
		}},
	}}}
}

func (h Handler) taskQuestionAction(w http.ResponseWriter, r *http.Request, eventID string, raw []byte) {
	toast := func(kind, text string) {
		_ = json.NewEncoder(w).Encode(map[string]any{"toast": map[string]string{"type": kind, "content": text}})
	}
	var event struct {
		Event struct {
			Host     string `json:"host"`
			Operator struct {
				Tenant string `json:"tenant_key"`
				OpenID string `json:"open_id"`
			} `json:"operator"`
			Context struct {
				Message string `json:"open_message_id"`
				Chat    string `json:"open_chat_id"`
			} `json:"context"`
			Action struct {
				Tag   string `json:"tag"`
				Name  string `json:"name"`
				Value struct {
					Ref string `json:"feedback_ref"`
				} `json:"value"`
				Form struct {
					Answer string `json:"answer"`
				} `json:"form_value"`
			} `json:"action"`
		} `json:"event"`
	}
	if json.Unmarshal(raw, &event) != nil {
		toast("error", "This question could not be opened.")
		return
	}
	e := event.Event
	ref, answer := e.Action.Value.Ref, e.Action.Form.Answer
	if e.Host != "im_message" || e.Operator.Tenant != h.Client.Config.TenantKey || e.Operator.OpenID == "" ||
		e.Action.Tag != "button" || e.Action.Name != taskQuestionSubmit || !strings.HasPrefix(ref, "feedback:") || len(ref) > 256 {
		toast("error", "This question could not be opened.")
		return
	}
	if !task.ValidFeedbackAnswer(answer) {
		toast("error", "Enter a valid answer before submitting.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2500*time.Millisecond)
	defer cancel()
	inbound := interaction.InboundEvent{Channel: "feishu", Subject: h.Client.Subject(e.Operator.OpenID), Destination: e.Context.Chat,
		EventID: eventID, SourceID: e.Context.Message, Content: conversation.Text(answer)}
	_, err := h.Core.RespondTaskFeedbackInbound(ctx, inbound, ref, answer)
	switch {
	case err == nil:
		if h.Wake != nil {
			h.Wake()
		}
		toast("success", "Answer saved. Ghost will review it.")
	case errors.Is(err, interaction.ErrUnauthorized), errors.Is(err, task.ErrNotFound):
		toast("error", "This question could not be opened.")
	case errors.Is(err, task.ErrFeedbackStale), errors.Is(err, conversation.ErrConflict), errors.Is(err, agent.ErrConflict):
		toast("error", "This question changed or already has a different answer. Check Tasks.")
	default:
		toast("error", "The answer could not be confirmed. Check Tasks before trying again.")
	}
}
