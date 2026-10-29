package web

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/z-chenhao/collie/internal/agent"
	"github.com/z-chenhao/collie/internal/conversation"
	"github.com/z-chenhao/collie/internal/interaction"
	"github.com/z-chenhao/collie/internal/task"
)

func TestAuthoritativeApprovalHasOneCardWithoutRewritingUserText(t *testing.T) {
	c := &updatesCore{allowed: true, status: "busy", approvals: []interaction.Approval{{ID: "one", Status: "pending", Tool: "builtin__write", Hash: strings.Repeat("a", 64), Arguments: `{"path":"report.txt","content":"review"}`}}, messages: []conversation.Message{{MessageID: "approval:one", Source: "security", Role: "assistant", Sequence: 1, Content: conversation.Text("Use /approve one PRIVATE_CLI_HASH")}, {MessageID: "user", Source: "web", Role: "user", Sequence: 2, Content: conversation.Text("Keep the literal /approve text I wrote.")}}}
	s := &Server{Core: c}
	w := httptest.NewRecorder()
	s.show(w, httptest.NewRequest("GET", "/", nil))
	body := w.Body.String()
	if w.Code != 200 || strings.Count(body, `data-approval="one"`) != 1 || strings.Contains(body, "PRIVATE_CLI_HASH") || !strings.Contains(body, "Keep the literal /approve text I wrote.") {
		t.Fatal(w.Code, body)
	}
	if !strings.Contains(body, `name="hash" value="`+strings.Repeat("a", 64)) {
		t.Fatal("presentation lost the exact frozen action")
	}
}

type inboundCore struct {
	updatesCore
	event interaction.InboundEvent
	calls int
}

func (c *inboundCore) AcceptInbound(_ context.Context, e interaction.InboundEvent) (agent.Receipt, error) {
	c.event = e
	c.calls++
	return agent.Receipt{MessageID: "accepted"}, nil
}
func TestTaskCommunicationEntersMainWithValidatedTaskContext(t *testing.T) {
	for _, missing := range []bool{false, true} {
		core := &inboundCore{}
		reader := workspaceReader{}
		if missing {
			reader.err = task.ErrNotFound
		}
		s := &Server{Core: core, Tasks: reader}
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		form.WriteField("source_id", "task-input")
		form.WriteField("text", "Please adjust the recommendation.")
		form.WriteField("task_ref", "task:visible")
		form.Close()
		r := httptest.NewRequest("POST", "/message", &body)
		r.Header.Set("Content-Type", form.FormDataContentType())
		r.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		err := s.acceptFormMessage(w, r)
		if missing {
			if err == nil || core.calls != 0 {
				t.Fatal("unavailable task reached ingress", err)
			}
			continue
		}
		if err != nil || core.calls != 1 || core.event.Channel != "web" || !strings.Contains(core.event.Content[0].Text, "task:visible") || !strings.Contains(core.event.Content[0].Text, "Please adjust the recommendation.") {
			t.Fatal(err, core.event)
		}
	}
}
