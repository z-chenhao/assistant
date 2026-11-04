package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestFeishuTaskFeedbackCardLifecycleFences(t *testing.T) {
	for _, mode := range []string{"revoke_during_token", "forget_during_token", "legacy_question_after_receipt", "review_missing_during_token", "review_missing_after_receipt", "review_forget_after_receipt", "review_rebind_after_receipt", "review_changed_origin"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			s, runs, a, p, taskRef := feedbackFixture(t)
			binding, err := s.BindChannel(ctx, "feishu", "tenant:app:owner", "chat", true, 0)
			if err != nil {
				t.Fatal(err)
			}
			requestTool := task.ManageRequest{Operation: "request_input", TaskRef: taskRef, Prompt: "Which period should I use?"}
			var artifactRef, workspace string
			if strings.HasPrefix(mode, "review_") {
				workspace = "https://owner.example.test"
				runs.ConfigureFeishuReviewWorkspace(workspace)
				artifactRef = putArtifact(t, s, artifactOwner(t, s), "Immutable revision")
				requestTool.Operation = "request_review"
				requestTool.ArtifactRefs = []string{artifactRef}
			}
			raw, _ := json.Marshal(requestTool)
			p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
				if req.Messages[len(req.Messages)-1].Role == "tool" {
					return llm.GenerationResult{Text: "Question saved."}, nil
				}
				return calls(task.ManageName, string(raw)), nil
			}
			input := interaction.InboundEvent{Channel: binding.Channel, Subject: binding.Subject, Destination: binding.Destination, EventID: "event", SourceID: "om_question_origin", Content: conversation.Text("Ask about the period")}
			if artifactRef != "" {
				input.Content = append(input.Content, conversation.ContentPart{Type: "file", ArtifactRef: artifactRef})
			}
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
			if mode == "review_changed_origin" {
				workspace = "https://changed.example.test"
			}
			client := feishu.NewClient(feishu.Config{TenantKey: "tenant", AppID: "app", AppSecretRef: "secret:app", APIBase: peer.URL, WorkspaceURL: workspace}, secrets)
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
			removeFile := func() {
				t.Helper()
				var key string
				if err := s.db.QueryRow(`SELECT storage_key FROM artifact WHERE artifact_ref=?`, artifactRef).Scan(&key); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(filepath.Dir(s.path), "artifacts", key)); err != nil {
					t.Fatal(err)
				}
			}
			switch mode {
			case "review_missing_during_token":
				removeFile()
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
			if !strings.HasSuffix(mode, "_after_receipt") {
				if observed.Outcome != delivery.Failed || replies.Load() != 0 || scalar(t, s, `SELECT count(*) FROM channel_receipt WHERE delivery_id=?`, request.DeliveryID) != 0 {
					t.Fatal("revoked card crossed the credential boundary", observed, replies.Load())
				}
				return
			}
			if observed.Outcome != delivery.Delivered || replies.Load() != 1 {
				t.Fatal("card was not actually accepted by the transport", observed)
			}
			if mode == "legacy_question_after_receipt" {
				// Emulate a 039 database with a question, its frozen projection,
				// and its receipt already persisted before the additive migration.
				if _, err := s.db.Exec(removeGitHubFailedCheckFixtureSchema + `ALTER TABLE feishu_task_question DROP COLUMN review_url; PRAGMA user_version=39`); err != nil {
					t.Fatal(err)
				}
				root := filepath.Dir(s.path)
				history, registry := runs.history, runs.registry
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
				s, err = Open(ctx, root)
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
				runs = s.Runs(history, registry)
				adapter.Store = s
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
			expected := task.ErrFeedbackStale
			switch mode {
			case "review_missing_after_receipt":
				removeFile()
			case "review_rebind_after_receipt":
				disabled, err := s.BindChannel(ctx, binding.Channel, binding.Subject, binding.Destination, false, binding.Version)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := s.BindChannel(ctx, binding.Channel, binding.Subject, binding.Destination, true, disabled.Version); err != nil {
					t.Fatal(err)
				}
				expected = interaction.ErrUnauthorized
			default:
				forget()
			}
			input.EventID = "after-revocation"
			if _, err := runs.RespondTaskFeedbackInbound(ctx, input, ref, "Use Q3."); !errors.Is(err, expected) {
				t.Fatal("card replay escaped the binding, privacy or live Artifact barrier", err)
			}
			if scalar(t, s, `SELECT count(*) FROM main_input WHERE request_key=?`, local.RequestKey) != 1 ||
				scalar(t, s, `SELECT count(*) FROM channel_callback WHERE event_id='after-revocation'`) != 0 {
				t.Fatal("stale callback mutated the ingress ledger")
			}
			if query := adapter.Query(ctx, envelope); query.Outcome != delivery.Delivered || replies.Load() != 1 {
				t.Fatal("privacy revocation erased the authoritative transport receipt", query)
			}
		})
	}
}
