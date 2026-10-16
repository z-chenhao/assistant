package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/event"
	eventsqlite "github.com/z-chenhao/erii/internal/event/sqlite"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/tool"
	sqlitedriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type Runs struct {
	host        *Store
	history     conversation.Store
	registry    *tool.Registry
	recentAge   time.Duration
	recentLimit int
}

func (s *Store) Runs(history conversation.Store, registry *tool.Registry) *Runs {
	return &Runs{host: s, history: history, registry: registry, recentAge: 30 * 24 * time.Hour, recentLimit: 200}
}

type claim struct {
	id, token string
	epoch     int64
}

func (c *claim) RunID() string { return c.id }

const leaseDuration = 5 * time.Second

func newID() string                 { return uuid.Must(uuid.NewV7()).String() }
func encoded(v any) (string, error) { b, e := llm.CanonicalJSON(v); return string(b), e }
func digest(v string) string        { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }
func runEvent(ctx context.Context, tx *sql.Tx, id, kind string, version int64) error {
	e, err := event.New("/erii/runs", "ai.erii."+kind+".v1", id, struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}{id, version})
	if err != nil {
		return err
	}
	return eventsqlite.Enqueue(ctx, tx, e)
}
func (r *Runs) write(ctx context.Context, fn func(*sql.Tx) error) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = r.host.Transaction(ctx, fn)
		var sqliteErr *sqlitedriver.Error
		if !errors.As(err, &sqliteErr) || (sqliteErr.Code()&0xff != sqlite3.SQLITE_BUSY && sqliteErr.Code()&0xff != sqlite3.SQLITE_LOCKED) || attempt == 2 {
			break
		}
		timer := time.NewTimer(time.Duration(10<<attempt) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return agent.ErrStorage
		case <-timer.C:
		}
	}
	if err == nil {
		return nil
	}
	for _, known := range []error{agent.ErrBusy, agent.ErrClaimLost, agent.ErrConflict, agent.ErrSetup, conversation.ErrConflict, llm.ModelIncompatible, llm.ContextLengthExceeded} {
		if errors.Is(err, known) {
			return known
		}
	}
	return agent.ErrStorage
}
func fence(ctx context.Context, tx *sql.Tx, c agent.RunClaim) error {
	owner, ok := c.(*claim)
	if !ok {
		return agent.ErrClaimLost
	}
	var valid int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM run_claim c JOIN run r USING(run_id) WHERE c.run_id=? AND c.token=? AND c.epoch=? AND c.expires_at>? AND r.status='running' AND (r.kind='main' OR EXISTS(SELECT 1 FROM task t WHERE t.task_id=r.task_id AND t.agent_id=r.agent_id AND t.task_epoch=r.task_epoch AND t.status='active'))`, owner.id, owner.token, owner.epoch, time.Now().UnixMicro()).Scan(&valid); err != nil {
		return agent.ErrStorage
	}
	if valid != 1 {
		return agent.ErrClaimLost
	}
	return nil
}

func (r *Runs) Submit(ctx context.Context, key string, parts []conversation.ContentPart) (agent.Receipt, error) {
	parts, err := conversation.Normalize(parts)
	if err != nil {
		return agent.Receipt{}, err
	}
	receipt := agent.Receipt{RequestKey: key}
	content, _ := encoded(parts)
	err = r.write(ctx, func(tx *sql.Tx) error {
		var agentID, profile, status string
		var configVersion int64
		if err := tx.QueryRowContext(ctx, `SELECT agent_id,coalesce(active_model_profile_id,''),setup_status,config_version FROM agent_config WHERE singleton=1`).Scan(&agentID, &profile, &status, &configVersion); err != nil {
			return err
		}
		var old string
		err := tx.QueryRowContext(ctx, `SELECT run_id,message_id,message_json FROM main_input WHERE agent_id=? AND request_key=?`, agentID, key).Scan(&receipt.RunID, &receipt.MessageID, &old)
		if err == nil {
			var message conversation.Message
			if json.Unmarshal([]byte(old), &message) != nil {
				return agent.ErrStorage
			}
			prior, _ := encoded(message.Content)
			if prior != content {
				return conversation.ErrConflict
			}
			return nil
		}
		if err != sql.ErrNoRows {
			return err
		}
		if profile == "" || status != "ready" {
			return agent.ErrSetup
		}
		err = tx.QueryRowContext(ctx, `SELECT run_id FROM run WHERE agent_id=? AND kind='main' AND status IN ('queued','running','waiting')`, agentID).Scan(&receipt.RunID)
		if err == sql.ErrNoRows {
			receipt.RunID, err = r.createMain(ctx, tx, agentID, profile, configVersion, nil)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		receipt.MessageID = newID()
		message := conversation.Message{MessageID: receipt.MessageID, ConversationID: agentID, Role: "user", Content: parts, Source: "cli", SourceEventID: key, CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
		raw, _ := encoded(message)
		if _, err = tx.ExecContext(ctx, `INSERT INTO main_input(agent_id,request_key,run_id,message_id,message_json) VALUES(?,?,?,?,?)`, agentID, key, receipt.RunID, receipt.MessageID, raw); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_intent(message_id,message_json) VALUES(?,?)`, receipt.MessageID, raw); err != nil {
			return err
		}
		return runEvent(ctx, tx, receipt.RunID, "main_input.accepted", 1)
	})
	return receipt, err
}

func (r *Runs) Next(ctx context.Context) (string, error) {
	if err := r.recoverStops(ctx); err != nil {
		return "", err
	}
	if err := r.Report(ctx); err != nil {
		return "", err
	}
	if err := r.promoteFollowups(ctx); err != nil {
		return "", err
	}
	r.host.boundary("before_next_scan")
	var id string
	// Four worker leases plus a reserved Main slot. The database enforces this
	// host bound across concurrent CLI processes as well as within one Actor.
	err := r.host.db.QueryRowContext(ctx, `SELECT r.run_id FROM run r JOIN run_claim c USING(run_id) WHERE (r.status IN ('queued','running') OR (r.status='waiting' AND (r.wait_reason='resource' OR (r.wait_reason='user_input' AND (EXISTS(SELECT 1 FROM main_input i WHERE i.run_id=r.run_id AND i.sequence>json_extract(r.checkpoint_json,'$.input_sequence')) OR EXISTS(SELECT 1 FROM task_instruction i WHERE i.task_id=r.task_id AND i.task_epoch=r.task_epoch AND i.sequence>json_extract(r.checkpoint_json,'$.input_sequence'))))))) AND (c.token IS NULL OR c.expires_at<=?) AND (r.kind='main' OR (SELECT count(*) FROM run w JOIN run_claim wc USING(run_id) WHERE w.kind='worker' AND w.status='running' AND wc.token IS NOT NULL AND wc.expires_at>?)<4) ORDER BY CASE r.kind WHEN 'main' THEN 0 ELSE 1 END,r.created_at,r.run_id LIMIT 1`, time.Now().UnixMicro(), time.Now().UnixMicro()).Scan(&id)
	if err == sql.ErrNoRows {
		var active int
		if err = r.host.db.QueryRowContext(ctx, `SELECT count(*) FROM run WHERE status IN ('queued','running') OR (status='waiting' AND wait_reason='resource')`).Scan(&active); err != nil {
			return "", agent.ErrStorage
		}
		if active > 0 {
			return "", agent.ErrBusy
		}
		// A terminal commit atomically removes the last runnable Root/Main and
		// may create an update or unblock an Inbox after our earlier scans.
		// Once active==0, any such completion must already be visible here.
		var pending int
		if err = r.host.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM task_update u JOIN task t USING(task_id) WHERE t.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1) AND u.task_epoch=t.task_epoch AND NOT EXISTS(SELECT 1 FROM event_inbox i WHERE i.consumer='task_reporter' AND i.event_id=u.event_id)) OR EXISTS(SELECT 1 FROM main_inbox b JOIN task_update u USING(event_id) JOIN task t USING(task_id) WHERE b.run_id IS NULL AND b.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1) AND u.task_epoch=t.task_epoch)`).Scan(&pending); err != nil {
			return "", agent.ErrStorage
		}
		if pending > 0 {
			return "", agent.ErrBusy
		}
		return "", agent.ErrNotFound
	}
	if err != nil {
		return "", agent.ErrStorage
	}
	return id, nil
}

func (r *Runs) Claim(ctx context.Context, id string) (agent.RunClaim, error) {
	c := &claim{id: id, token: newID()}
	err := r.write(ctx, func(tx *sql.Tx) error {
		now := time.Now().UnixMicro()
		var workers int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM run w JOIN run_claim wc USING(run_id) WHERE w.kind='worker' AND w.status='running' AND wc.token IS NOT NULL AND wc.expires_at>? AND EXISTS(SELECT 1 FROM run WHERE run_id=? AND kind='worker')`, now, id).Scan(&workers); err != nil {
			return err
		}
		if workers >= 4 {
			return agent.ErrBusy
		}
		res, err := tx.ExecContext(ctx, `UPDATE run_claim SET token=?,epoch=epoch+1,expires_at=?,version=version+1 WHERE run_id=? AND (token IS NULL OR expires_at<=?) AND EXISTS(SELECT 1 FROM run r WHERE run_id=? AND status IN ('queued','running','waiting') AND desired_state='running' AND (kind='main' OR EXISTS(SELECT 1 FROM task t WHERE t.task_id=r.task_id AND t.task_epoch=r.task_epoch AND t.status='active')))`, c.token, now+leaseDuration.Microseconds(), id, now, id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return agent.ErrBusy
		}
		if err = tx.QueryRowContext(ctx, `SELECT epoch FROM run_claim WHERE run_id=?`, id).Scan(&c.epoch); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE run SET status='running',wait_reason=NULL,started_at=coalesce(started_at,?),updated_at=?,version=version+1 WHERE run_id=?`, now, now, id); err != nil {
			return err
		}
		return runEvent(ctx, tx, id, "run.claimed", c.epoch)
	})
	return c, err
}
func (r *Runs) Renew(ctx context.Context, c agent.RunClaim) error {
	return r.write(ctx, func(tx *sql.Tx) error {
		if err := ownership(ctx, tx, c); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE run_claim SET expires_at=?,version=version+1 WHERE run_id=?`, time.Now().Add(leaseDuration).UnixMicro(), c.RunID())
		return err
	})
}
func (r *Runs) Release(ctx context.Context, c agent.RunClaim) error {
	return r.write(ctx, func(tx *sql.Tx) error {
		if err := fence(ctx, tx, c); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE run_claim SET token=NULL,expires_at=0,version=version+1 WHERE run_id=?`, c.RunID()); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE run SET updated_at=?,version=version+1 WHERE run_id=?`, time.Now().UnixMicro(), c.RunID()); err != nil {
			return err
		}
		return runEvent(ctx, tx, c.RunID(), "run.released", 1)
	})
}

func (r *Runs) Load(ctx context.Context, c agent.RunClaim) (agent.Run, error) {
	var run agent.Run
	var snapshotID, cp, tools string
	err := r.write(ctx, func(tx *sql.Tx) error {
		if err := fence(ctx, tx, c); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `SELECT r.run_id,r.agent_id,r.conversation_id,r.kind,r.status,coalesce(r.wait_reason,''),r.version,r.model_snapshot_id,r.checkpoint_json,c.tools_json FROM run r JOIN capability_snapshot c ON c.snapshot_id=r.capability_snapshot_id WHERE r.run_id=?`, c.RunID()).Scan(&run.ID, &run.AgentID, &run.ConversationID, &run.Kind, &run.Status, &run.WaitReason, &run.Version, &snapshotID, &cp, &tools)
	})
	if err != nil {
		return run, err
	}
	if json.Unmarshal([]byte(cp), &run.Checkpoint) != nil || json.Unmarshal([]byte(tools), &run.Tools) != nil {
		return run, agent.ErrStorage
	}
	run.Snapshot, err = r.host.Models().Snapshot(ctx, snapshotID)
	return run, err
}

// Flush applies only immutable accepted intents. Fencing is unnecessary here:
// any executor can fill missing writes, but cannot invent or change a message.
func (r *Runs) Flush(ctx context.Context) error {
	rows, err := r.host.db.QueryContext(ctx, `SELECT message_json FROM conversation_intent WHERE flushed=0 ORDER BY sequence`)
	if err != nil {
		return agent.ErrStorage
	}
	messages := []conversation.Message{}
	for rows.Next() {
		var raw string
		var m conversation.Message
		if rows.Scan(&raw) != nil || json.Unmarshal([]byte(raw), &m) != nil {
			rows.Close()
			return agent.ErrStorage
		}
		messages = append(messages, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return agent.ErrStorage
	}
	assistant := false
	for _, m := range messages {
		if m.Role == "assistant" {
			assistant = true
		}
	}
	if assistant {
		r.host.boundary("before_conversation_append")
	}
	if err = r.history.Append(ctx, messages); err != nil {
		return err
	}
	if assistant {
		r.host.boundary("after_conversation_append")
	}
	return r.write(ctx, func(tx *sql.Tx) error {
		for _, m := range messages {
			if _, err := tx.ExecContext(ctx, `UPDATE conversation_intent SET flushed=1 WHERE message_id=?`, m.MessageID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Runs) Initialize(ctx context.Context, c agent.RunClaim) error {
	if err := r.Flush(ctx); err != nil {
		return err
	}
	run, err := r.Load(ctx, c)
	if err != nil {
		return err
	}
	if run.Kind == "worker" {
		return nil
	}
	if run.Checkpoint.ContextReady {
		return nil
	}
	rows, err := r.host.db.QueryContext(ctx, `SELECT message_id FROM conversation_intent WHERE sequence<=(SELECT history_boundary FROM run WHERE run_id=?)`, c.RunID())
	if err != nil {
		return agent.ErrStorage
	}
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			return agent.ErrStorage
		}
		ids[id] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return agent.ErrStorage
	}
	history, err := r.history.List(ctx, run.ConversationID)
	if err != nil {
		return err
	}
	filtered := []conversation.Message{}
	for _, m := range history {
		if ids[m.MessageID] {
			filtered = append(filtered, m)
		}
	}
	runtimeMessages := []agent.AgentMessage{}
	base := []agent.AgentMessage{}
	for _, m := range run.Checkpoint.Messages {
		if m.Type == "custom" {
			runtimeMessages = append(runtimeMessages, m)
		} else {
			base = append(base, m)
		}
	}
	run.Checkpoint.Messages = base
	for _, m := range filtered {
		run.Checkpoint.Messages = append(run.Checkpoint.Messages, agent.AgentMessage{Type: m.Role, Content: m.Content})
	}
	run.Checkpoint.Messages = append(run.Checkpoint.Messages, runtimeMessages...)
	run.Checkpoint.ContextReady = true
	return r.Checkpoint(ctx, c, run.Version, run.Checkpoint)
}
func (r *Runs) Steering(ctx context.Context, c agent.RunClaim, after int64) ([]agent.Input, error) {
	run, err := r.Load(ctx, c)
	if err != nil {
		return nil, err
	}
	if run.Kind == "worker" {
		return r.taskSteering(ctx, c, after)
	}
	if err := r.Flush(ctx); err != nil {
		return nil, err
	}
	var inputs []agent.Input
	deferred := false
	err = r.write(ctx, func(tx *sql.Tx) error {
		inputs = nil
		deferred = false
		if err := fence(ctx, tx, c); err != nil {
			return err
		}
		// An accepted exchange owns this step's frozen context, even if its
		// result has not reached the Run checkpoint yet. Recover it before
		// injecting newly arrived steering, or request-hash drift would result.
		var frozen int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM model_generation g JOIN run r USING(run_id) WHERE g.run_id=? AND g.step_index=json_extract(r.checkpoint_json,'$.step')`, c.RunID()).Scan(&frozen); err != nil {
			return err
		}
		if frozen > 0 {
			return nil
		}
		rows, err := tx.QueryContext(ctx, `SELECT i.sequence,i.message_json FROM main_input i JOIN conversation_intent c USING(message_id) WHERE i.run_id=? AND i.sequence>? AND c.flushed=1 ORDER BY i.sequence`, c.RunID(), after)
		if err != nil {
			return err
		}

		for rows.Next() {
			var i agent.Input
			var raw string
			if rows.Scan(&i.Sequence, &raw) != nil || json.Unmarshal([]byte(raw), &i.Message) != nil {
				rows.Close()
				return agent.ErrStorage
			}
			inputs = append(inputs, i)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(inputs) > 0 {
			var generations int
			if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM model_generation WHERE run_id=?`, c.RunID()).Scan(&generations); err != nil {
				return err
			}
			if generations == 0 {
				cp := run.Checkpoint
				base := []agent.AgentMessage{}
				for _, m := range cp.Messages {
					if m.Type == "custom" {
						deferred = true
					} else {
						base = append(base, m)
					}
				}
				if deferred {
					cp.Messages = base
					if _, err = tx.ExecContext(ctx, `UPDATE main_inbox SET run_id=NULL WHERE run_id=?`, c.RunID()); err != nil {
						return err
					}
					return checkpoint(ctx, tx, c, run.Version, cp)
				}
			}
		}
		return nil
	})
	if err == nil && deferred {
		return nil, agent.ErrConflict
	}
	return inputs, err
}
func checkpoint(ctx context.Context, tx *sql.Tx, c agent.RunClaim, version int64, cp agent.Checkpoint) error {
	if err := fence(ctx, tx, c); err != nil {
		return err
	}
	raw, err := encoded(cp)
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE run SET checkpoint_json=?,checkpoint_ref=?,updated_at=?,version=version+1 WHERE run_id=? AND version=?`, raw, digest(raw), time.Now().UnixMicro(), c.RunID(), version)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return agent.ErrConflict
	}
	return runEvent(ctx, tx, c.RunID(), "run.checkpointed", version+1)
}
func (r *Runs) Checkpoint(ctx context.Context, c agent.RunClaim, version int64, cp agent.Checkpoint) error {
	for _, m := range cp.Messages {
		if _, err := conversation.Normalize(m.Content); err != nil {
			return err
		}
	}
	return r.write(ctx, func(tx *sql.Tx) error { return checkpoint(ctx, tx, c, version, cp) })
}

func (r *Runs) terminal(ctx context.Context, c agent.RunClaim, version int64, status, reason string) (bool, error) {
	done := false
	worker := false
	err := r.write(ctx, func(tx *sql.Tx) error {
		done = false
		if err := fence(ctx, tx, c); err != nil {
			return err
		}
		if status == "completed" {
			var phase string
			if err := tx.QueryRowContext(ctx, `SELECT json_extract(checkpoint_json,'$.phase') FROM run WHERE run_id=?`, c.RunID()).Scan(&phase); err != nil {
				return err
			}
			if phase != "settle" {
				return agent.ErrConflict
			}
			var pending int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM main_input i JOIN run r USING(run_id) WHERE i.run_id=? AND i.sequence>json_extract(r.checkpoint_json,'$.input_sequence')`, c.RunID()).Scan(&pending); err != nil {
				return err
			}
			if pending > 0 {
				return nil
			}
			var pendingTask int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM task_instruction i JOIN run r ON r.task_id=i.task_id AND r.task_epoch=i.task_epoch WHERE r.run_id=? AND i.sequence>json_extract(r.checkpoint_json,'$.input_sequence')`, c.RunID()).Scan(&pendingTask); err != nil {
				return err
			}
			if pendingTask > 0 {
				return nil
			}
		}
		now := time.Now().UnixMicro()
		res, err := tx.ExecContext(ctx, `UPDATE run SET status=?,error_code=?,ended_at=?,updated_at=?,version=version+1 WHERE run_id=? AND version=?`, status, reason, now, now, c.RunID(), version)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return agent.ErrConflict
		}
		if _, err = tx.ExecContext(ctx, `UPDATE run_claim SET token=NULL,expires_at=0,version=version+1 WHERE run_id=?`, c.RunID()); err != nil {
			return err
		}
		if err = r.finishTask(ctx, tx, c.RunID(), status, reason); err != nil {
			return err
		}
		done = true
		var workers int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM run WHERE run_id=? AND kind='worker'`, c.RunID()).Scan(&workers); err != nil {
			return err
		}
		worker = workers == 1
		if worker {
			r.host.boundary("before_result_commit")
		}
		return runEvent(ctx, tx, c.RunID(), "run."+status, version+1)
	})
	if err == nil && done && worker {
		r.host.boundary("after_result_commit")
	}
	return done, err
}
func (r *Runs) Complete(ctx context.Context, c agent.RunClaim, v int64) (bool, error) {
	return r.terminal(ctx, c, v, "completed", "")
}
func (r *Runs) Fail(ctx context.Context, c agent.RunClaim, v int64, code string) error {
	_, err := r.terminal(ctx, c, v, "failed", llm.NormalizeError(code).Error())
	return err
}
func (r *Runs) Wait(ctx context.Context, c agent.RunClaim, v int64, reason string) error {
	switch reason {
	case "user_input", "approval", "external_event", "resource":
	default:
		return agent.ErrConflict
	}
	return r.write(ctx, func(tx *sql.Tx) error {
		if err := fence(ctx, tx, c); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE run SET status='waiting',wait_reason=?,updated_at=?,version=version+1 WHERE run_id=? AND version=?`, reason, time.Now().UnixMicro(), c.RunID(), v)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return agent.ErrConflict
		}
		if _, err = tx.ExecContext(ctx, `UPDATE run_claim SET token=NULL,expires_at=0,version=version+1 WHERE run_id=?`, c.RunID()); err != nil {
			return err
		}
		return runEvent(ctx, tx, c.RunID(), "run.waiting", v+1)
	})
}
