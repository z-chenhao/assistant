package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/z-chenhao/erii/internal/agent"
	"time"
)

// Operations is an operator-only bounded projection. It deliberately excludes
// prompts, arguments, results, destinations, endpoint/account and credential refs.
type Operations struct {
	Version int                   `json:"version"`
	At      string                `json:"at"`
	Counts  map[string]int64      `json:"counts"`
	Limits  agent.SchedulerLimits `json:"scheduler_limits"`
	Trees   []TreeHealth          `json:"trees"`
	DLQ     []DeadEvent           `json:"dlq"`
}
type TreeHealth struct {
	Root          string `json:"root_run_id"`
	Ticket        int64  `json:"last_served"`
	Live          int64  `json:"live_workers"`
	Queued        int64  `json:"queued_workers"`
	OldestSeconds int64  `json:"oldest_queued_seconds"`
}
type DeadEvent struct {
	ID       string `json:"event_id"`
	Attempts int    `json:"attempts"`
	Failures int    `json:"failures"`
	Code     string `json:"code"`
}

func (s *Store) Operations(ctx context.Context) (Operations, error) {
	d := Operations{Version: 1, At: time.Now().UTC().Format(time.RFC3339Nano), Counts: map[string]int64{}, Limits: s.schedulerLimits(), Trees: []TreeHealth{}, DLQ: []DeadEvent{}}
	if !d.Limits.Valid() {
		d.Limits = agent.DefaultSchedulerLimits()
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return d, e
	}
	defer tx.Rollback()
	now := time.Now().UnixMicro()
	queries := map[string]string{
		"runs_active":                     `SELECT count(*) FROM run WHERE status IN ('queued','running','waiting')`,
		"trees_active":                    `SELECT count(DISTINCT root_run_id) FROM run WHERE kind='worker' AND status IN ('queued','running','waiting')`,
		"workers_live":                    `SELECT count(*) FROM run_claim c JOIN run r USING(run_id) WHERE r.kind='worker' AND c.token IS NOT NULL AND c.expires_at>:now`,
		"claims_expired":                  `SELECT count(*) FROM run_claim c JOIN run r USING(run_id) WHERE c.token IS NOT NULL AND c.expires_at<=:now AND r.status IN ('queued','running','waiting')`,
		"execution_claims_expired":        `SELECT count(*) FROM code_execution_state WHERE claim_token IS NOT NULL AND claim_expires_at<=:now AND state IN ('running','suspended')`,
		"wakes_pending":                   `SELECT count(*) FROM code_execution_state WHERE wake_seq>processed_wake_seq AND state IN ('running','suspended')`,
		"outbox_pending":                  `SELECT count(*) FROM event_outbox WHERE published_at IS NULL AND dead_at IS NULL`,
		"outbox_dead":                     `SELECT count(*) FROM event_outbox WHERE dead_at IS NOT NULL`,
		"outbox_attempts":                 `SELECT coalesce(sum(attempt_count),0) FROM event_outbox`,
		"conversation_intents_pending":    `SELECT count(*) FROM conversation_intent WHERE flushed=0`,
		"approvals_expired_pending":       `SELECT count(*) FROM approval WHERE status='pending' AND expires_at<=:now`,
		"approvals_pending":               `SELECT count(*) FROM approval WHERE status='pending'`,
		"spend_approvals_pending":         `SELECT count(*) FROM spend_approval WHERE status='pending'`,
		"automations_due":                 `SELECT count(*) FROM automation WHERE enabled=1 AND next_fire_at<=:now`,
		"tools_unknown":                   `SELECT count(*) FROM tool_invocation WHERE status='unknown'`,
		"deliveries_unknown":              `SELECT count(*) FROM delivery_request WHERE status='unknown'`,
		"deliveries_pending":              `SELECT count(*) FROM delivery_request WHERE status IN ('pending','sending','retry')`,
		"delivery_claims_expired":         `SELECT count(*) FROM delivery_outbox WHERE claim_token IS NOT NULL AND claim_expires_at<=:now`,
		"providers_circuit_open":          `SELECT count(*) FROM scheduler_provider WHERE blocked_until>:now`,
		"provider_infrastructure_streaks": `SELECT coalesce(sum(consecutive_errors),0) FROM scheduler_provider`,
	}
	for name, query := range queries {
		var n int64
		if e = tx.QueryRowContext(ctx, query, sql.Named("now", now)).Scan(&n); e != nil {
			return d, e
		}
		d.Counts[name] = n
	}
	rows, e := tx.QueryContext(ctx, `SELECT q.root_run_id,q.last_served,(SELECT count(*) FROM run r JOIN run_claim c USING(run_id) WHERE r.root_run_id=q.root_run_id AND r.kind='worker' AND c.token IS NOT NULL AND c.expires_at>?), (SELECT count(*) FROM run r WHERE r.root_run_id=q.root_run_id AND r.status='queued'),coalesce((?- (SELECT min(created_at) FROM run r WHERE r.root_run_id=q.root_run_id AND r.status='queued'))/1000000,0) FROM scheduler_tree q WHERE EXISTS(SELECT 1 FROM run active WHERE active.root_run_id=q.root_run_id AND active.status IN ('queued','running','waiting')) ORDER BY q.last_served,q.root_run_id LIMIT 100`, now, now)
	if e != nil {
		return d, e
	}
	for rows.Next() {
		var h TreeHealth
		if e = rows.Scan(&h.Root, &h.Ticket, &h.Live, &h.Queued, &h.OldestSeconds); e != nil {
			rows.Close()
			return d, e
		}
		d.Trees = append(d.Trees, h)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return d, e
	}
	rows, e = tx.QueryContext(ctx, `SELECT event_id,attempt_count,failure_count,failure_code FROM event_outbox WHERE dead_at IS NOT NULL ORDER BY dead_at,event_id LIMIT 100`)
	if e != nil {
		return d, e
	}
	for rows.Next() {
		var v DeadEvent
		if e = rows.Scan(&v.ID, &v.Attempts, &v.Failures, &v.Code); e != nil {
			rows.Close()
			return d, e
		}
		d.DLQ = append(d.DLQ, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return d, e
	}
	return d, tx.Commit()
}

type TracePoint struct {
	Sequence int64  `json:"sequence"`
	Kind     string `json:"kind"`
	At       string `json:"occurred_at"`
	Tool     string `json:"tool_call_id,omitempty"`
}
type RunTrace struct {
	RunID  string       `json:"run_id"`
	Parent string       `json:"parent_run_id,omitempty"`
	Root   string       `json:"root_run_id,omitempty"`
	Status string       `json:"status"`
	Points []TracePoint `json:"points"`
}

func (s *Store) OperationalTrace(ctx context.Context, id string) (RunTrace, error) {
	v := RunTrace{RunID: id, Points: []TracePoint{}}
	if e := s.db.QueryRowContext(ctx, `SELECT coalesce(parent_run_id,''),coalesce(root_run_id,''),status FROM run WHERE run_id=?`, id).Scan(&v.Parent, &v.Root, &v.Status); e != nil {
		return v, agent.ErrNotFound
	}
	rows, e := s.db.QueryContext(ctx, `SELECT sequence,event_json,occurred_at FROM run_trace WHERE run_id=? ORDER BY sequence DESC LIMIT 1000`, id)
	if e != nil {
		return v, e
	}
	defer rows.Close()
	for rows.Next() {
		var p TracePoint
		var raw []byte
		var at int64
		if e = rows.Scan(&p.Sequence, &raw, &at); e != nil {
			return v, e
		}
		p.At = time.UnixMicro(at).UTC().Format(time.RFC3339Nano)
		var event agent.Event
		if json.Unmarshal(raw, &event) != nil {
			return v, agent.ErrStorage
		}
		p.Kind = event.Kind
		p.Tool = event.ToolCallID
		v.Points = append(v.Points, p)
	}
	return v, rows.Err()
}
