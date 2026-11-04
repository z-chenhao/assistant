package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/z-chenhao/assistant/internal/conversation"
	"github.com/z-chenhao/assistant/internal/delivery"
	"github.com/z-chenhao/assistant/internal/interaction"
	"github.com/z-chenhao/assistant/internal/interaction/feishu"
	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/llm/memory"
	"github.com/z-chenhao/assistant/internal/task"
)

func TestFeishuTaskQuestionLifecycleFences(t *testing.T) {
	for _, mode := range []string{"revoke_during_token", "forget_during_token", "forget_after_receipt"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			s, runs, a, p, taskRef := feedbackFixture(t)
			binding, err := s.BindChannel(ctx, "feishu", "tenant:app:owner", "chat", true, 0)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(task.ManageRequest{Operation: "request_input", TaskRef: taskRef, Prompt: "Which period should I use?"})
			p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
				if req.Messages[len(req.Messages)-1].Role == "tool" {
					return llm.GenerationResult{Text: "Question saved."}, nil
				}
				return calls(task.ManageName, string(raw)), nil
			}
			input := interaction.InboundEvent{Channel: binding.Channel, Subject: binding.Subject, Destination: binding.Destination, EventID: "event", SourceID: "om_question_origin", Content: conversation.Text("Ask about the period")}
			if _, err := runs.AcceptInbound(ctx, input); err != nil {
				t.Fatal(err)
			}
			mustDrain(t, a)
			if err := runs.Flush(ctx); err != nil {
				t.Fatal(err)
			}
			var noticeID, ref string
			if err := s.db.QueryRow(`SELECT message_id,feedback_ref FROM feishu_task_question`).Scan(&noticeID, &ref); err != nil {
				t.Fatal(err)
			}
			var request delivery.Request
			for _, d := range deliveryRows(t, s) {
				if d.MessageID == noticeID {
					request = d
				}
			}
			msg, err := runs.history.Get(ctx, request.ConversationID, request.MessageID)
			if err != nil {
				t.Fatal(err)
			}
			envelope, err := delivery.Compose(request, msg)
			if err != nil {
				t.Fatal(err)
			}

			tokenStarted := make(chan struct{})
			releaseToken := make(chan struct{})
			var replies atomic.Int32
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/open-apis/auth/v3/tenant_access_token/internal":
					close(tokenStarted)
					select {
					case <-releaseToken:
						_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"test-access","expire":3600}`))
					case <-r.Context().Done():
					}
				case "/open-apis/im/v1/messages/om_question_origin/reply":
					replies.Add(1)
					_, _ = w.Write([]byte(`{"code":0,"data":{"message_id":"om_question_card"}}`))
				default:
					t.Error("unexpected platform request", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer peer.Close()
			defer cancel()
			secrets := memory.New()
			if err := secrets.Put(ctx, "secret:app", []byte("test-app-secret")); err != nil {
				t.Fatal(err)
			}
			client := feishu.NewClient(feishu.Config{TenantKey: "tenant", AppID: "app", AppSecretRef: "secret:app", APIBase: peer.URL}, secrets)
			adapter := feishu.Adapter{Client: *client, Store: s}
			done := make(chan delivery.Observation, 1)
			go func() { done <- adapter.Send(ctx, envelope) }()
			select {
			case <-tokenStarted:
			case <-ctx.Done():
				t.Fatal("credential request did not start")
			}
			agentID := idAgent(t, s)
			forget := func() {
				t.Helper()
				if err := s.Transaction(ctx, func(tx *sql.Tx) error { return runs.revokeMemory(ctx, tx, agentID) }); err != nil {
					t.Fatal(err)
				}
			}
			switch mode {
			case "revoke_during_token":
				if _, err := s.BindChannel(ctx, binding.Channel, binding.Subject, binding.Destination, false, binding.Version); err != nil {
					t.Fatal(err)
				}
			case "forget_during_token":
				forget()
			}
			close(releaseToken)
			var observed delivery.Observation
			select {
			case observed = <-done:
			case <-ctx.Done():
				t.Fatal("card delivery did not settle")
			}
			if mode != "forget_after_receipt" {
				if observed.Outcome != delivery.Failed || replies.Load() != 0 || scalar(t, s, `SELECT count(*) FROM channel_receipt WHERE delivery_id=?`, request.DeliveryID) != 0 {
					t.Fatal("revoked card crossed the credential boundary", observed, replies.Load())
				}
				return
			}
			if observed.Outcome != delivery.Delivered || replies.Load() != 1 {
				t.Fatal("card was not actually accepted by the transport", observed)
			}
			input.SourceID = "om_question_card"
			input.EventID = "answer"
			input.Content = conversation.Text("Use Q3.")
			local, err := runs.RespondTaskFeedback(ctx, ref, "Use Q3.")
			if err != nil {
				t.Fatal(err)
			}
			if replay, err := runs.RespondTaskFeedbackInbound(ctx, input, ref, "Use Q3."); err != nil || replay != local ||
				scalar(t, s, `SELECT count(*) FROM message_route WHERE message_id=?`, local.MessageID) != 0 {
				t.Fatal("Feishu retry changed the CLI receipt or route", replay, err)
			}
			forget()
			input.EventID = "after-forget"
			if _, err := runs.RespondTaskFeedbackInbound(ctx, input, ref, "Use Q3."); !errors.Is(err, task.ErrFeedbackStale) {
				t.Fatal("card replay escaped the privacy barrier", err)
			}
			if scalar(t, s, `SELECT count(*) FROM main_input WHERE request_key=?`, local.RequestKey) != 1 ||
				scalar(t, s, `SELECT count(*) FROM channel_callback WHERE event_id='after-forget'`) != 0 {
				t.Fatal("stale callback mutated the ingress ledger")
			}
			if query := adapter.Query(ctx, envelope); query.Outcome != delivery.Delivered || replies.Load() != 1 {
				t.Fatal("privacy revocation erased the authoritative transport receipt", query)
			}
		})
	}
}
