package sqlite

import (
	"context"
	"encoding/json"

	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/task"
)

func (r *Reader) history(ctx context.Context, out *task.Detail, after string) error {
	scope := "history:" + out.TaskRef
	c, err := decode(after, scope)
	if err != nil {
		return err
	}
	rows, err := r.db.QueryContext(ctx, `WITH events AS (
 SELECT 'instruction:'||i.instruction_id id,'instruction' kind,i.instruction text,i.created_at at FROM task_instruction i JOIN task t USING(task_id) WHERE t.task_ref=? AND t.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1)
 UNION ALL SELECT u.event_id,u.kind,'',u.occurred_at FROM task_update u JOIN task t USING(task_id) WHERE t.task_ref=? AND t.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1)
 ) SELECT id,kind,text,at FROM events WHERE (?='' OR at<? OR (at=? AND id<?)) ORDER BY at DESC,id DESC LIMIT 21`, out.TaskRef, out.TaskRef, after, c.At, c.At, c.Ref)
	if err != nil {
		return err
	}
	defer rows.Close()
	var lastAt int64
	var lastID string
	for rows.Next() {
		var id string
		var at int64
		var v task.History
		if err = rows.Scan(&id, &v.Kind, &v.Text, &at); err != nil {
			return err
		}
		if len(out.History) == 20 {
			out.HistoryNext = encode(scope, lastAt, lastID)
			break
		}
		v.ID = id
		v.At = stamp(at)
		out.History = append(out.History, v)
		lastAt, lastID = at, id
	}
	return rows.Err()
}

func (r *Reader) messages(ctx context.Context, out *task.Detail) error {
	// Related Main deliveries only. Raw Root/Child return values never become a
	// separate user conversation or silently stand in for Main's final delivery.
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT c.sequence,c.message_json FROM task_update u JOIN task t USING(task_id) JOIN main_inbox i USING(event_id) JOIN main_reply m ON m.run_id=i.run_id JOIN conversation_intent c ON json_extract(c.message_json,'$.message_id')=m.message_id WHERE t.task_ref=? AND t.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1) AND json_extract(c.message_json,'$.conversation_id')=t.conversation_id ORDER BY c.sequence DESC LIMIT 10`, out.TaskRef)
	if err != nil {
		return err
	}
	for rows.Next() {
		var seq int64
		var raw string
		var m conversation.Message
		if err = rows.Scan(&seq, &raw); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal([]byte(raw), &m); err != nil {
			rows.Close()
			return err
		}
		m.Sequence = seq
		out.Messages = append(out.Messages, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	approvals, err := r.db.QueryContext(ctx, `SELECT a.approval_id FROM approval a JOIN tool_invocation i USING(invocation_id) JOIN run rr USING(run_id) JOIN task t USING(task_id) WHERE t.task_ref=? AND t.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1) AND a.status='pending'
 UNION ALL SELECT a.approval_id FROM spend_approval a JOIN run rr ON rr.run_id=a.root_run_id JOIN task t USING(task_id) WHERE t.task_ref=? AND t.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1) AND a.status='pending'`, out.TaskRef, out.TaskRef)
	if err != nil {
		return err
	}
	for approvals.Next() {
		var id string
		if err = approvals.Scan(&id); err != nil {
			approvals.Close()
			return err
		}
		out.ApprovalIDs = append(out.ApprovalIDs, id)
	}
	err = approvals.Err()
	approvals.Close()
	if err != nil {
		return err
	}
	return r.db.QueryRowContext(ctx, `SELECT coalesce(min(c.sequence),0) FROM task_request q JOIN task t ON json_extract(q.receipt_json,'$.taskRef')=t.task_ref JOIN conversation_intent c ON json_extract(c.message_json,'$.message_id')=q.source_message_id AND json_extract(c.message_json,'$.conversation_id')=t.conversation_id WHERE t.task_ref=? AND t.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1)`, out.TaskRef).Scan(&out.SourceSequence)
}
