package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/tool"
)

type tools struct {
	runs  *Runs
	claim agent.RunClaim
}

func (r *Runs) ToolStore(c agent.RunClaim) tool.Store { return &tools{r, c} }

func (t *tools) Accept(ctx context.Context, exchange string, calls []llm.ToolCall, specs []tool.Spec) ([]tool.Invocation, error) {
	out := []tool.Invocation{}
	err := t.runs.write(ctx, func(tx *sql.Tx) error {
		out = out[:0]
		if err := fence(ctx, tx, t.claim); err != nil {
			return err
		}
		var frozen string
		if err := tx.QueryRowContext(ctx, `SELECT c.tools_json FROM run r JOIN capability_snapshot c ON c.snapshot_id=r.capability_snapshot_id WHERE r.run_id=?`, t.claim.RunID()).Scan(&frozen); err != nil {
			return err
		}
		current, _ := encoded(specs)
		if frozen != current {
			return llm.ModelIncompatible
		}
		// Only calls in a persisted complete model response can be accepted.
		var value string
		if err := tx.QueryRowContext(ctx, `SELECT result_json FROM model_generation WHERE generation_id=? AND run_id=? AND status='completed'`, exchange, t.claim.RunID()).Scan(&value); err != nil {
			return err
		}
		var response llm.GenerationResult
		if json.Unmarshal([]byte(value), &response) != nil {
			return agent.ErrStorage
		}
		a, _ := encoded(response.ToolCalls)
		b, _ := encoded(calls)
		if a != b {
			return agent.ErrConflict
		}
		for i, call := range calls {
			request, _ := encoded(call)
			hash := digest(request)
			var inv tool.Invocation
			var oldHash string
			var result sql.NullString
			err := tx.QueryRowContext(ctx, `SELECT invocation_id,status,request_hash,result_json FROM tool_invocation WHERE run_id=? AND provider_exchange_id=? AND provider_call_id=?`, t.claim.RunID(), exchange, call.ID).Scan(&inv.ID, &inv.Status, &oldHash, &result)
			inv.Call = call
			if err == nil {
				if oldHash != hash {
					return agent.ErrConflict
				}
				if result.Valid && json.Unmarshal([]byte(result.String), &inv.Result) != nil {
					return agent.ErrStorage
				}
			} else if err == sql.ErrNoRows {
				inv.ID = newID()
				inv.Status = "queued"
				now := time.Now().UnixMicro()
				policy := "never"
				for _, spec := range specs {
					if spec.Name == call.Name {
						policy = spec.ReplayPolicy
					}
				}
				_, err = tx.ExecContext(ctx, `INSERT INTO tool_invocation(invocation_id,run_id,provider_exchange_id,provider_call_id,tool_name,arguments_json,request_hash,replay_policy,idempotency_key,status,delivery_seq,created_at,updated_at,version) VALUES(?,?,?,?,?,?,?,?,?,'queued',?,?,?,1)`, inv.ID, t.claim.RunID(), exchange, call.ID, call.Name, call.Arguments, hash, policy, newID(), i, now, now)
				if err != nil {
					return err
				}
				if err = runEvent(ctx, tx, t.claim.RunID(), "tool_invocation.accepted", 1); err != nil {
					return err
				}
			} else {
				return err
			}
			out = append(out, inv)
		}
		return nil
	})
	return out, err
}
func (t *tools) Start(ctx context.Context, id string) error {
	return t.runs.write(ctx, func(tx *sql.Tx) error {
		if err := fence(ctx, tx, t.claim); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE tool_invocation SET status='running',attempt_count=attempt_count+1,version=version+1,updated_at=? WHERE invocation_id=? AND run_id=? AND (status='queued' OR (status='running' AND replay_policy='safe'))`, time.Now().UnixMicro(), id, t.claim.RunID())
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return agent.ErrConflict
		}
		return runEvent(ctx, tx, t.claim.RunID(), "tool_invocation.running", 1)
	})
}
func (t *tools) Finish(ctx context.Context, id string, result tool.Result) error {
	err := t.runs.write(ctx, func(tx *sql.Tx) error {
		if err := fence(ctx, tx, t.claim); err != nil {
			return err
		}
		value, err := encoded(result)
		if err != nil {
			return err
		}
		status := "succeeded"
		var errorJSON any
		if result.Error != nil {
			status = "failed"
			errorJSON, _ = encoded(result.Error)
		}
		now := time.Now().UnixMicro()
		res, err := tx.ExecContext(ctx, `UPDATE tool_invocation SET status=?,result_json=?,result_ref=?,error_json=?,result_ready_at=?,updated_at=?,version=version+1 WHERE invocation_id=? AND run_id=? AND status='running'`, status, value, digest(value), errorJSON, now, now, id, t.claim.RunID())
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return agent.ErrConflict
		}
		t.runs.host.boundary("before_tool_commit")
		return runEvent(ctx, tx, t.claim.RunID(), "tool_invocation."+status, 1)
	})
	if err == nil {
		t.runs.host.boundary("after_tool_commit")
	}
	return err
}
