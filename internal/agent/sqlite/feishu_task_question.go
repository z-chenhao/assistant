package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/z-chenhao/assistant/internal/conversation"
	"github.com/z-chenhao/assistant/internal/interaction"
	"github.com/z-chenhao/assistant/internal/task"
)

// Only a Main question derived from an owner-authored Feishu input can reserve
// an interactive notice. The route and binding version commit with the question.
func saveFeishuTaskQuestion(ctx context.Context, tx *sql.Tx, change *taskChange, source string) error {
	var binding, remote, title string
	var version int64
	err := tx.QueryRowContext(ctx, `SELECT b.binding_id,m.reply_source_id,b.version,t.title
 FROM main_input i JOIN message_route m USING(message_id) JOIN channel_binding b USING(binding_id)
 JOIN channel_preference p ON p.singleton=1 JOIN task t ON t.task_id=?
 WHERE i.message_id=? AND i.agent_id=t.agent_id AND b.channel='feishu' AND b.enabled=1 AND m.reply_source_id!=''
 AND json_extract(i.message_json,'$.role')='user' AND json_extract(i.message_json,'$.source')='feishu'
 AND json_extract(i.message_json,'$.source_event_id')=m.reply_source_id
 AND (p.channel='auto' OR (p.channel='feishu' AND p.binding_id=b.binding_id))`, change.id, source).Scan(&binding, &remote, &version, &title)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	id := "feishu-question:" + change.feedbackRef
	message := conversation.Message{MessageID: id, ConversationID: change.conversationID, Role: "assistant",
		Content: conversation.Text("Question about " + title + "\n\n" + change.request.Prompt),
		Source:  "task_feedback", SourceEventID: id, CreatedAt: time.UnixMicro(change.at).UTC()}
	raw, err := encoded(message)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_intent(message_id,message_json) VALUES(?,?)`, id, raw); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO message_route(message_id,binding_id,reply_source_id) VALUES(?,?,?)`, id, binding, remote); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO notification_intent VALUES(?,'needs_input')`, id); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO feishu_task_question VALUES(?,?,?)`, change.feedbackRef, id, version)
	return err
}

// An answered question may replay its accepted receipt after the normal Task
// continuation advances the epoch. A new answer always needs the original epoch.
const feishuQuestionCurrent = `f.status IN ('pending','answered')
 AND (f.task_epoch=t.task_epoch OR (f.status='answered' AND EXISTS(
  SELECT 1 FROM task_instruction i WHERE i.task_id=t.task_id AND i.task_epoch=t.task_epoch AND i.source_message_id=f.response_message_id
 )))
 AND t.status IN ('active','completed') AND t.explicit_end_epoch<t.task_epoch
 AND f.privacy_epoch=coalesce(p.epoch,0) AND t.created_at>coalesce(p.updated_at,0) AND f.expires_at>?
 AND NOT EXISTS(SELECT 1 FROM task_feedback newer WHERE newer.task_id=f.task_id AND newer.rowid>f.rowid)`

// FeishuTaskQuestion reads the card association for both send and query. A
// settled transport receipt remains authoritative even after the form expires.
func (s *Store) FeishuTaskQuestion(ctx context.Context, messageID string) (string, bool, error) {
	var ref string
	var pending bool
	err := s.db.QueryRowContext(ctx, `SELECT q.feedback_ref,
 f.status='pending' AND b.enabled=1 AND b.version=q.binding_version AND (`+feishuQuestionCurrent+`)
 FROM feishu_task_question q JOIN task_feedback f USING(feedback_ref) JOIN task t USING(task_id)
 LEFT JOIN memory_privacy p USING(agent_id) JOIN message_route m USING(message_id) JOIN channel_binding b USING(binding_id)
 WHERE q.message_id=?`, time.Now().UnixMicro(), messageID).Scan(&ref, &pending)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return ref, pending, err
}

// Authorize from the exact remote card receipt, not from the untrusted action
// value alone. Check this in the same transaction as dedup and acceptance.
func requireFeishuTaskQuestion(ctx context.Context, tx *sql.Tx, e interaction.InboundEvent, ref string) error {
	var current bool
	err := tx.QueryRowContext(ctx, `SELECT (`+feishuQuestionCurrent+`)
 FROM feishu_task_question q JOIN task_feedback f USING(feedback_ref) JOIN task t USING(task_id)
 LEFT JOIN memory_privacy p USING(agent_id) JOIN message_route m USING(message_id) JOIN channel_binding b USING(binding_id)
 JOIN delivery_request d ON d.message_id=q.message_id JOIN channel_receipt c ON c.delivery_id=d.delivery_id AND c.part=0
 WHERE q.feedback_ref=? AND c.remote_id=? AND d.channel='feishu' AND d.intent='needs_input' AND d.destination_ref=b.destination
 AND t.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1) AND d.conversation_id=t.conversation_id
 AND b.channel='feishu' AND b.enabled=1 AND b.version=q.binding_version AND b.subject=? AND b.destination=?`,
		time.Now().UnixMicro(), ref, e.SourceID, e.Subject, e.Destination).Scan(&current)
	if err == sql.ErrNoRows {
		return interaction.ErrUnauthorized
	}
	if err != nil {
		return err
	}
	if !current {
		return task.ErrFeedbackStale
	}
	return nil
}
