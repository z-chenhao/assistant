package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/task"
	"github.com/z-chenhao/erii/internal/tool"
)

// createMain is shared by user ingress and durable Reporter follow-ups. Both
// read current profile/config only when creating a new Run.
func (r *Runs) createMain(ctx context.Context, tx *sql.Tx, agentID, profile string, configVersion int64, updates []agent.AgentMessage) (string, error) {
	var pv int64
	if err := tx.QueryRowContext(ctx, `SELECT version FROM model_profile WHERE profile_id=? AND status='ready'`, profile).Scan(&pv); err != nil {
		return "", agent.ErrSetup
	}
	snapshot, err := r.host.Models().FreezeProfile(ctx, tx, profile, pv)
	if err != nil {
		return "", err
	}
	id, capID := newID(), newID()
	specs, _ := encoded(r.registry.Describe())
	now := time.Now().UnixMicro()
	if _, err = tx.ExecContext(ctx, `INSERT INTO capability_snapshot VALUES(?,?,'main',?,?,?,0,?,?)`, capID, configVersion, digest(specs), digest("[]"), agentID, specs, now); err != nil {
		return "", err
	}
	cp, _ := encoded(agent.Checkpoint{Phase: "model", Messages: append(agent.MainContext(nil), updates...)})
	if _, err = tx.ExecContext(ctx, `INSERT INTO run(run_id,agent_id,conversation_id,kind,status,capability_snapshot_id,model_snapshot_id,checkpoint_json,history_boundary,created_at,updated_at,version) VALUES(?,?,?,'main','queued',?,?,?,(SELECT coalesce(max(sequence),0) FROM conversation_intent),?,?,1)`, id, agentID, agentID, capID, snapshot.SnapshotID, cp, now, now); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO run_claim VALUES(?,0,NULL,0,1)`, id); err != nil {
		return "", err
	}
	return id, runEvent(ctx, tx, id, "run_ready", 1)
}

func (r *Runs) createRoot(ctx context.Context, tx *sql.Tx, taskID string) error {
	var agentID, conversationID, title, brief, profile string
	var epoch, config, pv int64
	if err := tx.QueryRowContext(ctx, `SELECT t.agent_id,t.conversation_id,t.title,t.anchor_task,t.task_epoch,a.active_model_profile_id,a.config_version,p.version FROM task t JOIN agent_config a ON a.agent_id=t.agent_id JOIN model_profile p ON p.profile_id=a.active_model_profile_id WHERE t.task_id=? AND t.status='active' AND a.setup_status='ready' AND p.status='ready'`, taskID).Scan(&agentID, &conversationID, &title, &brief, &epoch, &profile, &config, &pv); err != nil {
		return agent.ErrSetup
	}
	snapshot, err := r.host.Models().FreezeProfile(ctx, tx, profile, pv)
	if err != nil {
		return err
	}
	specs := []tool.Spec{}
	for _, spec := range r.registry.Describe() {
		if spec.Name != task.SearchName && spec.Name != task.ManageName {
			specs = append(specs, spec)
		}
	}
	raw, _ := encoded(specs)
	id, capID := newID(), newID()
	now := time.Now().UnixMicro()
	if _, err = tx.ExecContext(ctx, `INSERT INTO capability_snapshot VALUES(?,?,'worker',?,?,?,0,?,?)`, capID, config, digest(raw), digest("[]"), agentID, raw, now); err != nil {
		return err
	}
	// Prior instructions remain part of the durable Task context on continuation;
	// no Conversation or Main checkpoint is copied into a Root.
	messages := agent.RootContext(title, brief)
	rows, err := tx.QueryContext(ctx, `SELECT instruction FROM task_instruction WHERE task_id=? AND task_epoch<? ORDER BY sequence`, taskID, epoch)
	if err != nil {
		return err
	}
	for rows.Next() {
		var text string
		if err = rows.Scan(&text); err != nil {
			rows.Close()
			return err
		}
		messages = append(messages, agent.TextMessage("user", "Previous Task instruction: "+text))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = tx.QueryContext(ctx, `SELECT u.event_id,t.task_ref,u.kind,u.content,u.occurred_at FROM task_update u JOIN task t USING(task_id) WHERE u.task_id=? AND u.task_epoch<? ORDER BY u.task_epoch,u.occurred_at,u.event_id`, taskID, epoch)
	if err != nil {
		return err
	}
	for rows.Next() {
		var eventID, ref, kind, content string
		var at int64
		if err = rows.Scan(&eventID, &ref, &kind, &content, &at); err != nil {
			rows.Close()
			return err
		}
		messages = append(messages, taskMessage(eventID, ref, kind, content, nil, time.UnixMicro(at).UTC()))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	cp, _ := encoded(agent.Checkpoint{ContextReady: true, Phase: "model", Messages: messages})
	if _, err = tx.ExecContext(ctx, `INSERT INTO run(run_id,agent_id,conversation_id,task_id,root_run_id,kind,worker_position,status,task_epoch,capability_snapshot_id,model_snapshot_id,checkpoint_json,history_boundary,created_at,updated_at,version) VALUES(?,?,?,?,?,'worker','root','queued',?,?,?,?,0,?,?,1)`, id, agentID, conversationID, taskID, id, epoch, capID, snapshot.SnapshotID, cp, now, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO run_claim VALUES(?,0,NULL,0,1)`, id); err != nil {
		return err
	}
	return runEvent(ctx, tx, id, "run_ready", 1)
}

func (t *tools) ExecuteBuiltin(ctx context.Context, inv tool.Invocation) (tool.Result, error) {
	result := tool.Failure("PERMISSION_DENIED")
	err := t.runs.write(ctx, func(tx *sql.Tx) error {
		if err := fence(ctx, tx, t.claim); err != nil {
			return err
		}
		// Resolve the accepted record rather than trusting invocation fields supplied
		// by a caller. Frozen role visibility is enforced again here.
		var kind, name, args, source string
		if err := tx.QueryRowContext(ctx, `SELECT r.kind,i.tool_name,i.arguments_json FROM tool_invocation i JOIN run r USING(run_id) WHERE i.invocation_id=? AND i.run_id=? AND i.status='running'`, inv.ID, t.claim.RunID()).Scan(&kind, &name, &args); err != nil {
			return err
		}
		if kind != "main" {
			return nil
		}
		var visible int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM run r JOIN capability_snapshot c ON c.snapshot_id=r.capability_snapshot_id, json_each(c.tools_json) j WHERE r.run_id=? AND json_extract(j.value,'$.name')=?`, t.claim.RunID(), name).Scan(&visible); err != nil {
			return err
		}
		if visible != 1 {
			return nil
		}
		if name == task.SearchName {
			request, err := task.ParseSearch(args)
			if err != nil {
				result = tool.Failure("INVALID_ARGUMENT")
				return nil
			}
			result, err = t.runs.searchTasks(ctx, tx, t.claim.RunID(), request)
			return err
		}
		if name != task.ManageName {
			return nil
		}
		request, err := task.ParseManage(args)
		if err != nil {
			result = tool.Failure("INVALID_ARGUMENT")
			return nil
		}
		// Artifact admission arrives with M3. Until then every nonempty reference is
		// inaccessible; never accept a fabricated/unvalidated reference.
		if len(request.ArtifactRefs) > 0 {
			result = tool.Failure("NOT_FOUND")
			return nil
		}
		if request.Operation == "pause" || request.Operation == "cancel" {
			result = tool.Failure("PERMISSION_DENIED")
			return nil
		} // ERII-12
		canonical, _ := encoded(request)
		hash := digest(canonical)
		var oldHash, receipt string
		err = tx.QueryRowContext(ctx, `SELECT request_hash,receipt_json FROM task_invocation WHERE invocation_id=?`, inv.ID).Scan(&oldHash, &receipt)
		if err == nil {
			if oldHash != hash {
				return agent.ErrConflict
			}
			result = tool.Result{Text: receipt}
			return nil
		}
		if err != sql.ErrNoRows {
			return err
		}
		// The source is the latest consumed input, never a late unconsumed Steering.
		if err = tx.QueryRowContext(ctx, `SELECT coalesce((SELECT i.message_id FROM main_input i WHERE i.run_id=r.run_id AND i.sequence<=json_extract(r.checkpoint_json,'$.input_sequence') ORDER BY i.sequence DESC LIMIT 1),(SELECT request_id FROM main_inbox b WHERE b.run_id=r.run_id ORDER BY b.sequence DESC LIMIT 1),'') FROM run r WHERE r.run_id=?`, t.claim.RunID()).Scan(&source); err != nil {
			return err
		}
		if source == "" {
			return agent.ErrConflict
		}
		err = tx.QueryRowContext(ctx, `SELECT receipt_json FROM task_request WHERE source_message_id=? AND request_hash=?`, source, hash).Scan(&receipt)
		if err == sql.ErrNoRows {
			var agentID, conv string
			if err = tx.QueryRowContext(ctx, `SELECT agent_id,conversation_id FROM run WHERE run_id=?`, t.claim.RunID()).Scan(&agentID, &conv); err != nil {
				return err
			}
			ref, taskID := request.TaskRef, ""
			epoch := int64(1)
			now := time.Now().UnixMicro()
			refs, _ := encoded(request.ArtifactRefs)
			if request.Operation == "create" {
				taskID, ref = newID(), "task:"+newID()
				if _, err = tx.ExecContext(ctx, `INSERT INTO task(task_id,task_ref,agent_id,conversation_id,title,anchor_task,anchor_artifact_refs,status,task_epoch,created_at,updated_at,version) VALUES(?,?,?,?,?,?,?,'active',1,?,?,1)`, taskID, ref, agentID, conv, task.Title(request.Task), request.Task, refs, now, now); err != nil {
					return err
				}
				if err = t.runs.createRoot(ctx, tx, taskID); err != nil {
					return err
				}
			} else {
				var status string
				var version int64
				if err = tx.QueryRowContext(ctx, `SELECT task_id,status,task_epoch,version FROM task WHERE task_ref=? AND agent_id=? AND conversation_id=?`, ref, agentID, conv).Scan(&taskID, &status, &epoch, &version); err == sql.ErrNoRows {
					result = tool.Failure("NOT_FOUND")
					return nil
				} else if err != nil {
					return err
				}
				var active int
				if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM run WHERE task_id=? AND worker_position='root' AND status IN ('queued','running','waiting')`, taskID).Scan(&active); err != nil {
					return err
				}
				fresh := status != "active" || active == 0
				if fresh {
					epoch++
				}
				res, err := tx.ExecContext(ctx, `UPDATE task SET status='active',task_epoch=?,completed_at=NULL,cancelled_at=NULL,updated_at=?,version=version+1 WHERE task_id=? AND version=?`, epoch, now, taskID, version)
				if err != nil {
					return err
				}
				n, _ := res.RowsAffected()
				if n != 1 {
					return agent.ErrConflict
				}
				if _, err = tx.ExecContext(ctx, `INSERT INTO task_instruction(instruction_id,task_id,task_epoch,source_message_id,instruction,artifact_refs,created_at) VALUES(?,?,?,?,?,?,?)`, newID(), taskID, epoch, source, request.Instruction, refs, now); err != nil {
					return err
				}
				if fresh {
					if err = t.runs.createRoot(ctx, tx, taskID); err != nil {
						return err
					}
				} else {
					if err = runEvent(ctx, tx, taskID, "task.steering", epoch); err != nil {
						return err
					}
				}
			}
			receipt, _ = encoded(task.Receipt{TaskRef: ref, Status: "active", Accepted: true})
			if _, err = tx.ExecContext(ctx, `INSERT INTO task_request VALUES(?,?,?,?)`, source, hash, receipt, inv.ID); err != nil {
				return err
			}
			if err = runEvent(ctx, tx, taskID, "task."+request.Operation, epoch); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO task_invocation VALUES(?,?,?)`, inv.ID, hash, receipt); err != nil {
			return err
		}
		result = tool.Result{Text: receipt}
		t.runs.host.boundary("before_task_commit")
		return nil
	})
	if err == nil && result.Error == nil {
		t.runs.host.boundary("after_task_commit")
	}
	return result, err
}

func (r *Runs) taskSteering(ctx context.Context, c agent.RunClaim, after int64) ([]agent.Input, error) {
	inputs := []agent.Input{}
	err := r.write(ctx, func(tx *sql.Tx) error {
		inputs = nil
		if err := fence(ctx, tx, c); err != nil {
			return err
		}
		var frozen int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM model_generation g JOIN run r USING(run_id) WHERE r.run_id=? AND g.step_index=json_extract(r.checkpoint_json,'$.step')`, c.RunID()).Scan(&frozen); err != nil {
			return err
		}
		if frozen > 0 {
			return nil
		}
		rows, err := tx.QueryContext(ctx, `SELECT i.sequence,i.instruction FROM task_instruction i JOIN run r ON r.task_id=i.task_id AND r.task_epoch=i.task_epoch WHERE r.run_id=? AND i.sequence>? ORDER BY i.sequence`, c.RunID(), after)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var i agent.Input
			var text string
			if err = rows.Scan(&i.Sequence, &text); err != nil {
				return err
			}
			i.Message.Content = conversation.Text("Task instruction: " + text)
			inputs = append(inputs, i)
		}
		return rows.Err()
	})
	return inputs, err
}

func (r *Runs) prepareWorkerReply(ctx context.Context, c agent.RunClaim, run agent.Run) (bool, error) {
	ready := false
	err := r.write(ctx, func(tx *sql.Tx) error {
		if err := fence(ctx, tx, c); err != nil {
			return err
		}
		var pending int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM task_instruction i JOIN run r ON r.task_id=i.task_id AND r.task_epoch=i.task_epoch WHERE r.run_id=? AND i.sequence>?`, run.ID, run.Checkpoint.InputSequence).Scan(&pending); err != nil {
			return err
		}
		if pending > 0 {
			return nil
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO worker_result VALUES(?,?,?,?) ON CONFLICT(exchange_id) DO NOTHING`, run.ID, run.Checkpoint.ExchangeID, run.Checkpoint.Result.Text, time.Now().UnixMicro()); err != nil {
			return err
		}
		ready = true
		return nil
	})
	return ready, err
}
func (r *Runs) deliverWorkerReply(ctx context.Context, c agent.RunClaim, run agent.Run) error {
	cp := run.Checkpoint
	cp.Step++
	cp.Phase = "settle"
	cp.Result = nil
	cp.ExchangeID = ""
	return r.Checkpoint(ctx, c, run.Version, cp)
}
