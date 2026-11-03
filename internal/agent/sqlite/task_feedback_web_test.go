package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/conversation"
	"github.com/z-chenhao/assistant/internal/interaction"
	"github.com/z-chenhao/assistant/internal/llm"
)

func TestTaskFeedbackWebIdentityAndCrossChannelReceiptKeepFirstRoute(t *testing.T) {
	for _, firstChannel := range []string{"cli", "web"} {
		t.Run(firstChannel, func(t *testing.T) {
			s, r, a, p, ref := feedbackFixture(t)
			f := requestFeedback(t, s, a, p, ref)
			ctx := context.Background()
			binding, err := s.BindChannel(ctx, "web", "browser", "local", true, 0)
			if err != nil {
				t.Fatal(err)
			}
			input := interaction.InboundEvent{Channel: "web", Subject: "browser", Destination: "local", EventID: "page-one", SourceID: "page-one", Content: conversation.Text("Use this period")}
			var first agent.Receipt
			if firstChannel == "cli" {
				first, err = r.RespondTaskFeedback(ctx, f.Ref, "Use this period")
			} else {
				first, err = r.RespondTaskFeedbackInbound(ctx, input, f.Ref, "Use this period")
			}
			if err != nil {
				t.Fatal(err)
			}
			if retry, err := r.RespondTaskFeedback(ctx, f.Ref, "Use this period"); err != nil || retry != first {
				t.Fatal(retry, err)
			}
			for _, eventID := range []string{"page-one", "page-two"} {
				input.EventID = eventID
				if retry, err := r.RespondTaskFeedbackInbound(ctx, input, f.Ref, "Use this period"); err != nil || retry != first {
					t.Fatal(retry, err)
				}
			}
			if scalar(t, s, `SELECT count(*) FROM main_input WHERE message_id=?`, first.MessageID) != 1 || scalar(t, s, `SELECT count(*) FROM channel_source WHERE request_key=?`, first.RequestKey) != 1 || scalar(t, s, `SELECT count(*) FROM channel_callback`) != 2 {
				t.Fatal("cross-channel retries duplicated input/source")
			}
			wantRoute := 0
			if firstChannel == "web" {
				wantRoute = 1
			}
			if scalar(t, s, `SELECT count(*) FROM message_route WHERE message_id=? AND binding_id=?`, first.MessageID, binding.ID) != wantRoute || scalar(t, s, `SELECT count(*) FROM main_input WHERE message_id=? AND json_extract(message_json,'$.source')=?`, first.MessageID, firstChannel) != 1 {
				t.Fatal("retry replaced original source/route")
			}
			input.EventID = "different-answer"
			if _, err := r.RespondTaskFeedbackInbound(ctx, input, f.Ref, "Different period"); err == nil {
				t.Fatal("different answer accepted")
			}
			p.respond = func(llm.GenerationRequest) (llm.GenerationResult, error) {
				return llm.GenerationResult{Text: "I received the response."}, nil
			}
			mustDrain(t, a)
			channel := "tui"
			if firstChannel == "web" {
				channel = "web"
			}
			if scalar(t, s, `SELECT count(*) FROM delivery_request d JOIN main_reply m USING(message_id) WHERE m.run_id=? AND d.channel=?`, first.RunID, channel) != 1 {
				t.Fatal("actual Main reply lost first source route")
			}
			if _, err := s.BindChannel(ctx, "web", "browser", "local", false, binding.Version); err != nil {
				t.Fatal(err)
			}
			input.EventID = "revoked-retry"
			if _, err := r.RespondTaskFeedbackInbound(ctx, input, f.Ref, "Use this period"); !errors.Is(err, interaction.ErrUnauthorized) {
				t.Fatal("revoked binding recovered a duplicate receipt", err)
			}
		})
	}
}
