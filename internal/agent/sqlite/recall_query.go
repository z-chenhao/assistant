package sqlite

import (
	"context"
	"encoding/json"
	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/conversation"
	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/recall"
	"strings"
	"time"
)

// Automatic recall uses real input, not the generated Task instruction block.
// Exclusions are Host provenance, never model-supplied authority or a text guess.
func (r *Runs) automaticRecallQuery(ctx context.Context, run agent.Run, messages []llm.Message) (recall.Query, error) {
	q := recall.Query{}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && messages[i].Origin != "runtime" {
			q.Query = messages[i].Text
			break
		}
	}
	for _, m := range messages {
		if m.Source != nil && strings.HasPrefix(m.Source.Ref, "conversation:") {
			q.ExcludeRefs = append(q.ExcludeRefs, m.Source.Ref)
		}
	}
	rows, e := r.host.db.QueryContext(ctx, `SELECT message_id,message_json FROM main_input WHERE run_id=? ORDER BY sequence`, run.ID)
	if e != nil {
		return q, agent.ErrStorage
	}
	for rows.Next() {
		var id, raw string
		if rows.Scan(&id, &raw) != nil {
			rows.Close()
			return q, agent.ErrStorage
		}
		q.ExcludeRefs = append(q.ExcludeRefs, "conversation:"+id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return q, agent.ErrStorage
	}
	if run.Kind == "worker" {
		rows, e = r.host.db.QueryContext(ctx, `SELECT c.message_id,c.message_json,coalesce(i.sequence,0) FROM conversation_intent c JOIN (
    SELECT q.source_message_id AS id,0 AS sequence FROM task_request q JOIN tool_invocation v USING(invocation_id) WHERE json_extract(v.arguments_json,'$.operation')='create' AND json_extract(q.receipt_json,'$.taskRef')=(SELECT t.task_ref FROM task t JOIN run r ON r.task_id=t.task_id WHERE r.run_id=?)
    UNION SELECT i.source_message_id,i.sequence FROM task_instruction i JOIN run r ON r.task_id=i.task_id WHERE r.run_id=? AND i.source_message_id IS NOT NULL
  ) i ON i.id=c.message_id ORDER BY i.sequence,c.sequence`, run.ID, run.ID)
		if e != nil {
			return q, agent.ErrStorage
		}
		for rows.Next() {
			var id, raw string
			var sequence int64
			if rows.Scan(&id, &raw, &sequence) != nil {
				rows.Close()
				return q, agent.ErrStorage
			}
			q.ExcludeRefs = append(q.ExcludeRefs, "conversation:"+id)
			if sequence <= run.Checkpoint.InputSequence {
				var m conversation.Message
				if json.Unmarshal([]byte(raw), &m) != nil {
					rows.Close()
					return q, agent.ErrStorage
				}
				if m.Role == "user" {
					text := ""
					for _, p := range m.Content {
						if p.Type == "text" {
							text += p.Text + "\n"
						}
					}
					if strings.TrimSpace(text) != "" {
						q.Query = text
					}
				}
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return q, agent.ErrStorage
		}
	}
	if len(q.Query) > 4096 {
		q.Query = string([]rune(q.Query)[:min(len([]rune(q.Query)), 1000)])
	}
	upper := time.Now().UTC()
	q.TimeRange = &recall.TimeRange{End: &upper}
	return r.privacyQuery(ctx, run.AgentID, q)
}
