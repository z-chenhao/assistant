package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/z-chenhao/collie/internal/agent"
	"github.com/z-chenhao/collie/internal/artifact"
	"github.com/z-chenhao/collie/internal/conversation"
	"github.com/z-chenhao/collie/internal/memory"
	"github.com/z-chenhao/collie/internal/recall"
	"github.com/z-chenhao/collie/internal/tool"
)

type MemoryWriter interface {
	AllowCategory(context.Context, recall.Scope, string, time.Time, string) error
	ForgetCategory(context.Context, recall.Scope, string, time.Time, string) error
	CheckRef(context.Context, recall.Scope, string, bool) error
	Curate(context.Context, string, recall.Scope, memory.SourceRef, time.Time, []string, bool, string) error
	Forget(context.Context, recall.Scope, string, time.Time) error
}

func (r *Runs) ConfigureMemory(w MemoryWriter, v memory.Vault) { r.memory = w; r.vault = v }

// Source observations contain references only; originals retain their domain
// ownership. The shared Outbox supplies wakeups, the table is the durable scan.
func (r *Runs) observeMemory(ctx context.Context, tx *sql.Tx, agentID, sourceType, ref string) error {
	if r.memory == nil {
		return nil
	}
	_, e := r.queueMemory(ctx, tx, agentID, "observe", sourceType, ref, "")
	return e
}
func (r *Runs) queueMemory(ctx context.Context, tx *sql.Tx, agentID, operation, typ, ref, payload string) (string, error) {
	var id string
	e := tx.QueryRowContext(ctx, `SELECT event_id FROM memory_mutation WHERE operation=? AND source_type=? AND source_ref=?`, operation, typ, ref).Scan(&id)
	if e == nil {
		return id, nil
	}
	if e != sql.ErrNoRows {
		return "", e
	}
	id = newID()
	now := time.Now().UnixMicro()
	if _, e = tx.ExecContext(ctx, `INSERT INTO memory_mutation(event_id,agent_id,operation,source_type,source_ref,payload,accepted_at) VALUES(?,?,?,?,?,?,?)`, id, agentID, operation, typ, ref, payload, now); e != nil {
		return "", e
	}
	return id, runEvent(ctx, tx, id, "memory_mutation.accepted", 1)
}

func (t *tools) manageMemory(ctx context.Context, inv tool.Invocation) (tool.Result, error) {
	result := tool.Failure("PERMISSION_DENIED")
	e := t.runs.write(ctx, func(tx *sql.Tx) error {
		if e := t.fence(ctx, tx); e != nil {
			return e
		}
		var args, kind, agentID string
		var visible int
		if e := tx.QueryRowContext(ctx, `SELECT i.arguments_json,r.kind,r.agent_id,(SELECT count(*) FROM capability_snapshot c,json_each(c.tools_json) j WHERE c.snapshot_id=r.capability_snapshot_id AND json_extract(j.value,'$.name')=?) FROM tool_invocation i JOIN run r USING(run_id) WHERE i.invocation_id=? AND i.run_id=? AND i.status='running' AND i.tool_name=?`, memory.ManageName, inv.ID, t.claim.RunID(), memory.ManageName).Scan(&args, &kind, &agentID, &visible); e != nil {
			return e
		}
		if kind != "main" || visible != 1 {
			return nil
		}
		if t.runs.memory == nil {
			result = tool.Failure("PROVIDER_UNAVAILABLE")
			return nil
		}
		m, ok := memory.Parse(args)
		if !ok {
			result = tool.Failure("INVALID_ARGUMENT")
			return nil
		}
		check := m.MemoryRef
		if check == "" {
			check = m.Supersedes
		}
		if check != "" {
			if e := t.runs.memory.CheckRef(ctx, recall.Scope{UserID: artifact.LocalUser, AgentID: agentID}, strings.TrimPrefix(check, "memory:"), m.Operation == "forget"); e != nil {
				result = tool.Failure("NOT_FOUND")
				return nil
			}
		}
		payload := args
		if m.Content != "" && (t.runs.vault.Ref != "" || memory.Sensitive(m.Content)) {
			var e error
			payload, e = t.runs.vault.Seal(ctx, args, inv.ID)
			if e != nil {
				result = tool.Failure("PROVIDER_UNAVAILABLE")
				return nil
			}
		}
		id, e := t.runs.queueMemory(ctx, tx, agentID, m.Operation, "feedback", inv.ID, payload)
		if e != nil {
			return e
		}
		// The privacy barrier and durable intent commit together. It is stronger
		// than replay reuse: old snapshots remain audit-only, never rewritten.
		if m.Operation == "forget" {
			if e = t.runs.revokeMemory(ctx, tx, agentID); e != nil {
				return e
			}
		}
		raw, _ := encoded(map[string]any{"accepted": true, "operation": m.Operation, "receipt": "memory-event:" + id})
		result = tool.Result{Text: raw}
		return nil
	})
	return result, e
}

func (r *Runs) revokeMemory(ctx context.Context, tx *sql.Tx, agentID string) error {
	now := time.Now().UnixMicro()
	if _, e := tx.ExecContext(ctx, `INSERT INTO memory_privacy(agent_id,epoch,history_floor,updated_at) VALUES(?,1,(SELECT coalesce(max(sequence),0) FROM conversation_intent),?) ON CONFLICT(agent_id) DO UPDATE SET epoch=epoch+1,history_floor=excluded.history_floor,updated_at=excluded.updated_at`, agentID, now); e != nil {
		return e
	}
	// Conservative scope invalidation covers prompt copies, compacted summaries,
	// saved Tool results and Child results without guessing semantic dependence.
	if _, e := tx.ExecContext(ctx, `UPDATE run SET desired_state='memory_forgotten',updated_at=?,version=version+1 WHERE agent_id=? AND status IN('queued','running','waiting') AND desired_state='running'`, now, agentID); e != nil {
		return e
	}
	if _, e := tx.ExecContext(ctx, `UPDATE tool_invocation SET desired_state='cancelled',updated_at=?,version=version+1 WHERE run_id IN(SELECT run_id FROM run WHERE agent_id=? AND desired_state='memory_forgotten') AND status IN('queued','running','waiting') AND desired_state!='cancelled'`, now, agentID); e != nil {
		return e
	}
	return runEvent(ctx, tx, agentID, "memory.revoked", now)
}

// CurateMemory processes a bounded serial batch. A crash after memory.db commit
// and before the state receipt repeats the SAME event; memory.db deduplicates it.
// A transient failure keeps the head event pending and emits no user delivery.
func (r *Runs) CurateMemory(ctx context.Context) error {
	if r.memory == nil {
		return nil
	}
	for n := 0; n < 32; n++ {
		var id, agentID, op, typ, ref, payload string
		var at, revoked int64
		e := r.host.db.QueryRowContext(ctx, `SELECT e.event_id,e.agent_id,e.operation,e.source_type,e.source_ref,e.payload,e.accepted_at,coalesce(p.updated_at,0) FROM memory_mutation e LEFT JOIN memory_privacy p USING(agent_id) WHERE e.applied_at IS NULL AND e.failed_at IS NULL ORDER BY e.sequence LIMIT 1`).Scan(&id, &agentID, &op, &typ, &ref, &payload, &at, &revoked)
		if e == sql.ErrNoRows {
			return nil
		}
		if e != nil {
			return agent.ErrStorage
		}
		scope := recall.Scope{UserID: artifact.LocalUser, AgentID: agentID}
		if op == "observe" && at <= revoked {
			e = nil
		} else if op == "observe" {
			var content, author string
			sourceRef := typ + ":" + ref
			if typ == "conversation" {
				var raw string
				e = r.host.db.QueryRowContext(ctx, `SELECT message_json FROM conversation_intent WHERE message_id=?`, ref).Scan(&raw)
				var m conversation.Message
				if e == nil {
					e = json.Unmarshal([]byte(raw), &m)
					author = m.Role
					if m.CreatedAt.UnixMicro() <= revoked {
						author = "revoked"
					}
					for _, p := range m.Content {
						if p.Type == "text" {
							content += p.Text + "\n"
						}
					}
				}
			} else if typ == "task" {
				var created int64
				e = r.host.db.QueryRowContext(ctx, `SELECT w.content,w.created_at,t.task_ref FROM worker_result w JOIN run r USING(run_id) JOIN task t USING(task_id) WHERE r.run_id=? ORDER BY w.created_at DESC LIMIT 1`, ref).Scan(&content, &created, &sourceRef)
				author = "worker"
				if created <= revoked {
					author = "revoked"
				}
			} else if typ == "run" {
				var created int64
				e = r.host.db.QueryRowContext(ctx, `SELECT content,created_at FROM worker_result WHERE run_id=? ORDER BY created_at DESC LIMIT 1`, ref).Scan(&content, &created)
				author = "worker"
				if created <= revoked {
					author = "revoked"
				}
			}
			if e == nil {
				e = r.memory.Curate(ctx, id, scope, memory.SourceRef{Type: typ, Ref: sourceRef}, time.UnixMicro(at).UTC(), memory.Extract(content, author), false, "")
			}
		} else {
			if strings.HasPrefix(payload, "sealed:v1:") {
				payload, e = r.vault.Open(ctx, payload, ref)
			}
			m, valid := memory.Parse(payload)
			if e == nil && !valid {
				e = recall.ErrInvalid
			}
			if e == nil && op == "forget" {
				if m.Category != "" {
					e = r.memory.ForgetCategory(ctx, scope, m.Category, time.UnixMicro(at).UTC(), id)
				} else {
					e = r.memory.Forget(ctx, scope, strings.TrimPrefix(m.MemoryRef, "memory:"), time.UnixMicro(at).UTC())
				}
			}
			if e == nil && op == "remember" {
				if m.Category != "" {
					e = r.memory.AllowCategory(ctx, scope, m.Category, time.UnixMicro(at).UTC(), id)
				} else {
					e = r.memory.Curate(ctx, id, scope, memory.SourceRef{Type: "feedback", Ref: "feedback:" + ref}, time.UnixMicro(at).UTC(), []string{m.Content}, true, m.Supersedes)
				}
			}
		}
		if e != nil {
			if errors.Is(e, recall.ErrConflict) || errors.Is(e, recall.ErrInvalid) {
				if writeErr := r.write(ctx, func(tx *sql.Tx) error {
					_, e := tx.ExecContext(ctx, `UPDATE memory_mutation SET failed_at=?,last_error='MEMORY_CONFLICT',attempts=attempts+1 WHERE event_id=? AND applied_at IS NULL AND failed_at IS NULL`, time.Now().UnixMicro(), id)
					if e != nil {
						return e
					}
					return runEvent(ctx, tx, id, "memory_mutation.rejected", 1)
				}); writeErr != nil {
					return writeErr
				}
				continue
			}
			if _, writeErr := r.host.db.ExecContext(context.WithoutCancel(ctx), `UPDATE memory_mutation SET attempts=attempts+1,last_error='MEMORY_UNAVAILABLE' WHERE event_id=? AND applied_at IS NULL AND failed_at IS NULL`, id); writeErr != nil {
				return agent.ErrStorage
			}
			return e
		}
		r.host.boundary("after_memory_commit")
		r.host.boundary("after_memory_commit:" + id)
		if e = r.write(ctx, func(tx *sql.Tx) error {
			res, e := tx.ExecContext(ctx, `UPDATE memory_mutation SET applied_at=?,attempts=attempts+1 WHERE event_id=? AND applied_at IS NULL`, time.Now().UnixMicro(), id)
			if e != nil {
				return e
			}
			if count, _ := res.RowsAffected(); count == 0 {
				return nil
			}
			return runEvent(ctx, tx, id, "memory_mutation.applied", 1)
		}); e != nil {
			return e
		}
	}
	return nil
}

func (r *Runs) privacyQuery(ctx context.Context, agentID string, q recall.Query) (recall.Query, error) {
	var floor int64
	e := r.host.db.QueryRowContext(ctx, `SELECT updated_at FROM memory_privacy WHERE agent_id=?`, agentID).Scan(&floor)
	if e == sql.ErrNoRows {
		return q, nil
	}
	if e != nil {
		return q, agent.ErrStorage
	}
	start := time.UnixMicro(floor).UTC()
	if q.TimeRange == nil {
		q.TimeRange = &recall.TimeRange{}
	}
	if q.TimeRange.Start == nil || q.TimeRange.Start.Before(start) {
		q.TimeRange.Start = &start
	}
	return q, nil
}
