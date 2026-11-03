package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/conversation"
	"github.com/z-chenhao/assistant/internal/interaction"
	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/task"
)

func TestTaskRepliesKeepExactSourceAndRespectExplicitPreference(t *testing.T) {
	for _, preference := range []string{"auto", "tui"} {
		t.Run(preference, func(t *testing.T) {
			s, r, a, p := taskFixture(t)
			ctx := context.Background()
			b, err := s.BindChannel(ctx, "feishu", "tenant:app:owner", "bound-chat", true, 0)
			if err != nil {
				t.Fatal(err)
			}
			if preference == "tui" {
				if err := s.PreferChannel(ctx, "tui", "", 1); err != nil {
					t.Fatal(err)
				}
			}
			p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
				last := req.Messages[len(req.Messages)-1]
				if isRoot(req) || last.Role == "tool" || !strings.HasPrefix(last.Text, "topic ") {
					return llm.GenerationResult{Text: "committed result"}, nil
				}
				return calls(task.ManageName, fmt.Sprintf(`{"operation":"create","task":%q}`, last.Text)), nil
			}
			for _, source := range []string{"topic-a", "topic-b"} {
				if _, err := r.AcceptInbound(ctx, interaction.InboundEvent{Channel: b.Channel, Subject: b.Subject, Destination: b.Destination, EventID: source, SourceID: source, Content: conversation.Text("topic " + source)}); err != nil {
					t.Fatal(err)
				}
				mustDrain(t, a)
			}
			if scalar(t, s, `SELECT count(*) FROM task`) != 2 || scalar(t, s, `SELECT count(*) FROM delivery_request`) != 4 {
				t.Fatal("task delivery chain incomplete")
			}
			for _, request := range deliveryRows(t, s) {
				if request.Channel == "feishu" {
					source, err := s.DeliveryReplySource(ctx, request)
					if err != nil || source != "topic-a" && source != "topic-b" {
						t.Fatal(source, err)
					}
				}
			}
			want := 4
			if preference == "tui" {
				want = 2
			}
			if scalar(t, s, `SELECT count(*) FROM delivery_request WHERE channel='feishu'`) != want {
				t.Fatal("explicit preference was overridden")
			}
			if scalar(t, s, `SELECT count(*) FROM (SELECT m.run_id FROM main_reply m JOIN main_inbox i ON i.run_id=m.run_id JOIN task_update u USING(event_id) JOIN task t USING(task_id) JOIN task_request q ON json_extract(q.receipt_json,'$.taskRef')=t.task_ref JOIN message_route r ON r.message_id=q.source_message_id GROUP BY m.run_id HAVING count(DISTINCT r.reply_source_id)>1)`) != 0 {
				t.Fatal("different topics shared a semantic Main delivery")
			}
		})
	}
}

func TestPendingTopicGroupsRemainSeparateAcrossRestart(t *testing.T) {
	s, r, a, p := taskFixture(t)
	ctx := context.Background()
	b, err := s.BindChannel(ctx, "feishu", "tenant:app:owner", "chat", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
		last := req.Messages[len(req.Messages)-1]
		if isRoot(req) || last.Role == "tool" || !strings.HasPrefix(last.Text, "topic ") {
			return llm.GenerationResult{Text: "committed result"}, nil
		}
		return calls(task.ManageName, fmt.Sprintf(`{"operation":"create","task":%q}`, last.Text)), nil
	}
	// Execute accepted Main/Root claims using the existing real Loop helper,
	// deliberately leaving Next/Recover scheduling for the explicit boundary.
	// This produces two real terminal TaskUpdates before either is promoted.
	for _, source := range []string{"topic-a", "topic-b"} {
		receipt, err := r.AcceptInbound(ctx, interaction.InboundEvent{Channel: b.Channel, Subject: b.Subject, Destination: b.Destination, EventID: source, SourceID: source, Content: conversation.Text("topic " + source)})
		if err != nil {
			t.Fatal(err)
		}
		if err := executeTaskRunID(t, a, r, receipt.RunID); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.db.Query(`SELECT run_id FROM run WHERE kind='worker' ORDER BY created_at,run_id`)
	if err != nil {
		t.Fatal(err)
	}
	var roots []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		roots = append(roots, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(roots) != 2 {
		t.Fatal(roots, err)
	}
	for _, id := range roots {
		if err := executeTaskRunID(t, a, r, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Report(ctx); err != nil {
		t.Fatal(err)
	}
	if scalar(t, s, `SELECT count(*) FROM task_update`) != 2 || scalar(t, s, `SELECT count(*) FROM main_inbox WHERE run_id IS NULL`) != 2 {
		t.Fatal("fixture did not create simultaneous pending topics")
	}
	if err := r.promoteFollowups(ctx); err != nil {
		t.Fatal(err)
	}
	if scalar(t, s, `SELECT count(*) FROM main_inbox WHERE run_id IS NULL`) != 1 || scalar(t, s, `SELECT count(*) FROM main_inbox WHERE run_id IS NOT NULL`) != 1 || scalar(t, s, `SELECT count(*) FROM run WHERE kind='main' AND status NOT IN ('completed','failed','cancelled')`) != 1 {
		t.Fatal("first Main consumed more than one topic or lost the other")
	}
	var first string
	if err := s.db.QueryRow(`SELECT run_id FROM main_inbox WHERE run_id IS NOT NULL`).Scan(&first); err != nil {
		t.Fatal(err)
	}
	// Reopen the real database while the first Main is still queued. Recovery
	// must retain both its frozen assignment and the other pending topic.
	other, err := Open(ctx, filepath.Dir(s.path))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	r2 := other.Runs(r.history, a.Registry)
	a2 := a
	a2.Store = r2
	if err := r2.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if scalar(t, other, `SELECT count(*) FROM main_inbox WHERE run_id IS NULL`) != 1 || scalar(t, other, `SELECT count(*) FROM run WHERE kind='main' AND status NOT IN ('completed','failed','cancelled')`) != 1 {
		t.Fatal("restart mixed topics or bypassed single-Main fencing")
	}
	if err := executeTaskRunID(t, a2, r2, first); err != nil {
		t.Fatal(err)
	}
	if err := r2.promoteFollowups(ctx); err != nil {
		t.Fatal(err)
	}
	if scalar(t, other, `SELECT count(*) FROM main_inbox WHERE run_id IS NULL`) != 0 || scalar(t, other, `SELECT count(DISTINCT run_id) FROM main_inbox`) != 2 || scalar(t, other, `SELECT count(*) FROM run WHERE kind='main' AND status NOT IN ('completed','failed','cancelled')`) != 1 {
		t.Fatal("second topic was lost, duplicated or mixed")
	}
	var second string
	if err := other.db.QueryRow(`SELECT run_id FROM main_inbox WHERE run_id!=?`, first).Scan(&second); err != nil {
		t.Fatal(err)
	}
	if err := executeTaskRunID(t, a2, r2, second); err != nil {
		t.Fatal(err)
	}
	if err := r2.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if scalar(t, other, `SELECT count(*) FROM main_inbox`) != 2 || scalar(t, other, `SELECT count(*) FROM delivery_request`) != 4 || scalar(t, other, `SELECT count(*) FROM run WHERE kind='main' AND status NOT IN ('completed','failed','cancelled')`) != 0 {
		t.Fatal("topic drain did not converge exactly once")
	}
	for _, source := range []string{"topic-a", "topic-b"} {
		if scalar(t, other, `SELECT count(*) FROM delivery_request d JOIN message_route m USING(message_id) WHERE m.reply_source_id=?`, source) != 2 {
			t.Fatal("result left its original topic", source)
		}
	}
}

func TestReplySourceMigrationRetainsOldFrozenRoutesAndRollsBack(t *testing.T) {
	s, r, _, a, _ := loopFixture(t)
	ctx := context.Background()
	b, err := s.BindChannel(ctx, "feishu", "tenant:app:owner", "chat", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.AcceptInbound(ctx, interaction.InboundEvent{Channel: b.Channel, Subject: b.Subject, Destination: b.Destination, EventID: "old", SourceID: "old", Content: conversation.Text("old accepted work")}); err != nil {
		t.Fatal(err)
	}
	mustDrain(t, a)
	if _, err := s.db.Exec(removeGitHubContinuationFixtureSchema + `DROP TRIGGER message_reply_source; ALTER TABLE message_route DROP COLUMN reply_source_id; PRAGMA user_version=34`); err != nil {
		t.Fatal(err)
	}
	broken := append(append([]string{}, migrations[:34]...), feishuReplySourcesSQL+"; invalid SQL")
	if err := migrate(ctx, s.db, broken); err == nil {
		t.Fatal("broken migration accepted")
	}
	if scalar(t, s, `PRAGMA user_version`) != 34 || scalar(t, s, `SELECT count(*) FROM pragma_table_info('message_route') WHERE name='reply_source_id'`) != 0 {
		t.Fatal("partial migration committed")
	}
	if err := migrate(ctx, s.db, migrations); err != nil {
		t.Fatal(err)
	}
	if scalar(t, s, `SELECT count(*) FROM message_route WHERE reply_source_id!=''`) != 0 {
		t.Fatal("old accepted action was silently redirected to reply API")
	}
	if _, err := s.db.Exec(`UPDATE message_route SET reply_source_id='other'`); err == nil {
		t.Fatal("frozen source updated")
	}
}

func TestTaskFeedbackResponseKeepsCLIAndOriginalFeishuRoutesSeparate(t *testing.T) {
	s, r, a, p := taskFixture(t)
	ctx := context.Background()
	b, err := s.BindChannel(ctx, "feishu", "tenant:app:owner", "chat", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
		if isRoot(req) || isFollowup(req) || req.Messages[len(req.Messages)-1].Role == "tool" {
			return llm.GenerationResult{Text: "accepted result"}, nil
		}
		return calls(task.ManageName, `{"operation":"create","task":"Prepare the linked report","completionPolicy":"ongoing"}`), nil
	}
	if _, err := r.AcceptInbound(ctx, interaction.InboundEvent{Channel: b.Channel, Subject: b.Subject, Destination: b.Destination, EventID: "original-event", SourceID: "om_original", Content: conversation.Text("Prepare the linked report")}); err != nil {
		t.Fatal(err)
	}
	mustDrain(t, a)
	var ref string
	if err := s.db.QueryRow(`SELECT task_ref FROM task`).Scan(&ref); err != nil {
		t.Fatal(err)
	}
	f := requestFeedback(t, s, a, p, ref)
	answer, err := r.RespondTaskFeedback(ctx, f.Ref, "Use the confirmed scope.")
	if err != nil {
		t.Fatal(err)
	}
	p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
		if isRoot(req) || isFollowup(req) || req.Messages[len(req.Messages)-1].Role == "tool" {
			return llm.GenerationResult{Text: "Main delivers the update"}, nil
		}
		return calls(task.ManageName, fmt.Sprintf(`{"operation":"append","taskRef":%q,"feedbackRef":%q,"instruction":"Use the confirmed scope."}`, ref, f.Ref)), nil
	}
	mustDrain(t, a)
	if scalar(t, s, `SELECT count(*) FROM task_feedback WHERE status='answered' AND response_message_id=?`, answer.MessageID) != 1 || scalar(t, s, `SELECT count(*) FROM task_instruction`) != 1 {
		t.Fatal("feedback acceptance/continuation was lost in channel integration")
	}
	if scalar(t, s, `SELECT count(*) FROM delivery_request WHERE channel='feishu'`) != 3 || scalar(t, s, `SELECT count(*) FROM delivery_request WHERE channel='tui'`) != 2 {
		t.Fatal("CLI response inherited a remote route or Task result lost its origin")
	}
	for _, request := range deliveryRows(t, s) {
		if request.Channel == "feishu" {
			source, err := s.DeliveryReplySource(ctx, request)
			if err != nil || source != "om_original" {
				t.Fatal("logical feedback key became a Feishu source", source, err)
			}
		}
	}
	retry, err := r.RespondTaskFeedback(ctx, f.Ref, "Use the confirmed scope.")
	if err != nil || retry != answer {
		t.Fatal("accepted feedback retry changed its receipt", retry, err)
	}
	if scalar(t, s, `SELECT count(*) FROM delivery_request`) != 5 {
		t.Fatal("feedback retry created another notification")
	}
}
