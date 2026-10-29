package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/z-chenhao/collie/internal/agent"
	"github.com/z-chenhao/collie/internal/artifact"
)

// mainHandoff carries only the immediately preceding Main's working state and
// visible receipts. Unknown Main effects remain explicit across later requests.
// The privacy barrier excludes all evidence accepted before Forget.
func (r *Runs) mainHandoff(ctx context.Context, c agent.RunClaim, run agent.Run) ([]agent.AgentMessage, error) {
	out := []agent.AgentMessage{}
	err := r.write(ctx, func(tx *sql.Tx) error {
		out = nil
		if e := fence(ctx, tx, c); e != nil {
			return e
		}
		var prior, checkpoint string
		e := tx.QueryRowContext(ctx, `SELECT p.run_id,p.checkpoint_json FROM run p JOIN run current ON current.run_id=? WHERE p.agent_id=current.agent_id AND p.kind='main' AND p.status IN ('completed','failed') AND p.created_at<current.created_at AND p.created_at>coalesce((SELECT updated_at FROM memory_privacy WHERE agent_id=p.agent_id),0) ORDER BY p.created_at DESC,p.run_id DESC LIMIT 1`, run.ID).Scan(&prior, &checkpoint)
		if e != nil && e != sql.ErrNoRows {
			return e
		}
		if e == nil {
			var cp agent.Checkpoint
			if json.Unmarshal([]byte(checkpoint), &cp) != nil {
				return agent.ErrStorage
			}
			if state := cp.WorkingState; state != nil {
				scope, e := runScope(ctx, tx, prior)
				if e != nil {
					return e
				}
				_, e = r.host.Artifacts().DescribeTx(ctx, tx, scope, state.ArchiveRef, true)
				if e == nil {
					// Incremental summaries refer to older chunks. Carry the visible
					// private history chain, not merely its newest archive file.
					refs, e := tx.QueryContext(ctx, `SELECT a.artifact_ref FROM artifact a WHERE a.put_key LIKE 'context:%' AND a.created_at>coalesce((SELECT updated_at FROM memory_privacy WHERE agent_id=?),0) AND (a.run_id=? OR EXISTS(SELECT 1 FROM artifact_reference g WHERE g.artifact_ref=a.artifact_ref AND g.holder_kind='run' AND g.holder_id=?))`, run.AgentID, prior, prior)
					if e != nil {
						return e
					}
					chain := []string{}
					for refs.Next() {
						var ref string
						if e = refs.Scan(&ref); e != nil {
							refs.Close()
							return e
						}
						chain = append(chain, ref)
					}
					e = refs.Err()
					refs.Close()
					if e != nil {
						return e
					}
					for _, ref := range chain {
						if e = r.host.Artifacts().PinTx(ctx, tx, scope, ref, "run", run.ID); e != nil && e != artifact.ErrNotFound {
							return e
						}
					}
					out = append(out, agent.RuntimeContext("previous_main_state", state.ArchiveRef, false, struct {
						Summary string `json:"summary"`
						History string `json:"history_artifact"`
					}{state.Text, state.ArchiveRef}))
				} else if e != artifact.ErrNotFound {
					return e
				}
			}
			rows, e := tx.QueryContext(ctx, `SELECT invocation_id,tool_name,status,result_json FROM tool_invocation WHERE run_id=? AND parent_invocation_id IS NULL AND status IN ('succeeded','failed') ORDER BY created_at,delivery_seq,invocation_id`, prior)
			if e != nil {
				return e
			}
			for rows.Next() {
				var id, name, status string
				var result sql.NullString
				if e = rows.Scan(&id, &name, &status, &result); e != nil {
					rows.Close()
					return e
				}
				if result.Valid {
					out = append(out, agent.RuntimeContext("previous_main_receipt", "effect:"+id, false, struct {
						Tool   string          `json:"tool"`
						Status string          `json:"status"`
						Result json.RawMessage `json:"result"`
					}{name, status, json.RawMessage(result.String)}))
				}
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
		}
		rows, e := tx.QueryContext(ctx, `SELECT i.invocation_id,i.tool_name FROM tool_invocation i JOIN run p USING(run_id) WHERE p.agent_id=? AND p.kind='main' AND p.run_id!=? AND i.status='unknown' AND p.created_at>coalesce((SELECT updated_at FROM memory_privacy WHERE agent_id=p.agent_id),0) ORDER BY p.created_at,i.created_at,i.invocation_id`, run.AgentID, run.ID)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var id, name string
			if e = rows.Scan(&id, &name); e != nil {
				return e
			}
			out = append(out, agent.RuntimeContext("unknown_effect", "effect:"+id, true, struct {
				Tool   string `json:"tool"`
				Status string `json:"status"`
			}{name, "unknown"}))
		}
		return rows.Err()
	})
	return out, err
}

// taskHandoff exposes final Root business results and Root-visible receipts,
// not the model's private intermediate responses or descendant tool histories.
func (r *Runs) taskHandoff(ctx context.Context, tx *sql.Tx, taskID string, epoch int64) ([]agent.AgentMessage, error) {
	out := []agent.AgentMessage{}
	// A pause can fence the Root after model acceptance but before report commit.
	// Preserve that final business text, excluding tool calls and internal summary
	// generations. A committed report supersedes this recovery representation.
	rows, err := tx.QueryContext(ctx, `SELECT content,result_id FROM (
 SELECT w.content,r.task_epoch,w.created_at,w.exchange_id AS result_id FROM worker_result w JOIN run r USING(run_id)
 WHERE r.task_id=? AND r.task_epoch<? AND r.worker_position='root'
 UNION ALL
 SELECT json_extract(g.result_json,'$.text'),r.task_epoch,g.completed_at,g.generation_id FROM model_generation g JOIN run r USING(run_id)
 WHERE r.task_id=? AND r.task_epoch<? AND r.worker_position='root' AND g.status='completed'
 AND coalesce(json_array_length(g.result_json,'$.tool_calls'),0)=0 AND length(trim(coalesce(json_extract(g.result_json,'$.text'),'')))>0
 AND NOT EXISTS(SELECT 1 FROM worker_result w WHERE w.run_id=r.run_id)
 AND NOT EXISTS(SELECT 1 FROM context_compaction cc WHERE cc.run_id=g.run_id AND cc.step_index=g.step_index AND json_extract(cc.provenance_json,'$.stage')='summary_request')
 ) ORDER BY task_epoch,created_at,result_id`, taskID, epoch, taskID, epoch)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var text, id string
		if err = rows.Scan(&text, &id); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, agent.RuntimeContext("task_result", "generation:"+id, false, text))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT i.invocation_id,i.tool_name,i.status,i.result_json FROM tool_invocation i JOIN run r USING(run_id) WHERE r.task_id=? AND r.task_epoch<? AND r.worker_position='root' AND i.parent_invocation_id IS NULL AND i.status IN ('succeeded','failed') ORDER BY r.task_epoch,i.created_at,i.delivery_seq,i.invocation_id`, taskID, epoch)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, name, status string
		var result sql.NullString
		if err = rows.Scan(&id, &name, &status, &result); err != nil {
			rows.Close()
			return nil, err
		}
		if !result.Valid {
			continue
		}
		out = append(out, agent.RuntimeContext("tool_receipt", "effect:"+id, false, struct {
			Tool   string          `json:"tool"`
			Status string          `json:"status"`
			Result json.RawMessage `json:"result"`
		}{name, status, json.RawMessage(result.String)}))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// Unknown effects are Host facts even when a descendant produced no explicit
	// output. Reveal no descendant arguments or intermediate result contents.
	rows, err = tx.QueryContext(ctx, `SELECT i.invocation_id,i.tool_name FROM tool_invocation i JOIN run r USING(run_id) WHERE r.task_id=? AND r.task_epoch<? AND i.status='unknown' ORDER BY r.task_epoch,i.created_at,i.invocation_id`, taskID, epoch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err = rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out = append(out, agent.RuntimeContext("unknown_effect", "effect:"+id, true, struct {
			Tool        string `json:"tool"`
			Status      string `json:"status"`
			Instruction string `json:"instruction"`
		}{name, "unknown", "Do not repeat this external operation. Obtain authoritative outcome evidence or ask the user to resolve it before proceeding with equivalent work."}))
	}
	return out, rows.Err()
}
