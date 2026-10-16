ALTER TABLE task ADD COLUMN completion_policy TEXT NOT NULL DEFAULT 'once' CHECK(completion_policy IN ('once','ongoing'));
ALTER TABLE tool_invocation ADD COLUMN desired_state TEXT NOT NULL DEFAULT 'running' CHECK(desired_state IN ('running','cancelled'));

DROP TRIGGER task_version;
CREATE TRIGGER task_version BEFORE UPDATE ON task WHEN NEW.version!=OLD.version+1 OR NEW.task_id!=OLD.task_id OR NEW.task_ref!=OLD.task_ref
 OR NEW.agent_id!=OLD.agent_id OR NEW.conversation_id!=OLD.conversation_id OR NEW.title!=OLD.title OR NEW.anchor_task!=OLD.anchor_task
 OR NEW.anchor_artifact_refs!=OLD.anchor_artifact_refs OR NEW.created_at!=OLD.created_at OR NEW.completion_policy!=OLD.completion_policy
 OR NEW.task_epoch!=OLD.task_epoch+CASE WHEN (NEW.status!=OLD.status AND NEW.status IN ('paused','cancelled')) OR (NEW.status='active' AND (OLD.status!='active' OR NEW.task_epoch!=OLD.task_epoch)) THEN 1 ELSE 0 END
 OR (NEW.status!=OLD.status AND NOT ((OLD.status='active' AND NEW.status IN ('paused','completed','cancelled')) OR (OLD.status='paused' AND NEW.status='cancelled') OR (OLD.status IN ('paused','completed','cancelled') AND NEW.status='active')))
BEGIN SELECT RAISE(ABORT,'invalid task transition'); END;

CREATE TRIGGER run_desired_state BEFORE UPDATE ON run WHEN NEW.desired_state NOT IN ('running','task_paused','task_cancelled')
 OR (OLD.desired_state!='running' AND NEW.desired_state='running')
BEGIN SELECT RAISE(ABORT,'invalid run stop intent'); END;

DROP TRIGGER invocation_transition;
CREATE TRIGGER invocation_transition BEFORE UPDATE ON tool_invocation WHEN NEW.version!=OLD.version+1 OR OLD.status IN ('succeeded','failed','cancelled') OR NEW.arguments_json!=OLD.arguments_json OR NEW.request_hash!=OLD.request_hash OR NEW.replay_policy!=OLD.replay_policy OR NEW.idempotency_key!=OLD.idempotency_key OR NEW.provider_call_id!=OLD.provider_call_id OR NEW.provider_exchange_id!=OLD.provider_exchange_id OR NEW.tool_name!=OLD.tool_name OR NEW.run_id!=OLD.run_id OR NEW.invocation_id!=OLD.invocation_id OR NEW.parent_invocation_id IS NOT OLD.parent_invocation_id
 OR (OLD.desired_state='cancelled' AND NEW.desired_state!='cancelled')
 OR NOT ((OLD.status='queued' AND NEW.status IN ('queued','running','cancelled')) OR (OLD.status='running' AND NEW.status IN ('running','succeeded','failed','cancelled','unknown')) OR (OLD.status='waiting' AND NEW.status IN ('waiting','cancelled','unknown')) OR (OLD.status='unknown' AND NEW.status IN ('succeeded','failed')))
BEGIN SELECT RAISE(ABORT,'invalid tool transition'); END;

-- A stop intent may be persisted on queued/waiting Runs without executing them.
DROP TRIGGER run_version;
CREATE TRIGGER run_version BEFORE UPDATE ON run WHEN NEW.version!=OLD.version+1 OR OLD.status IN ('completed','failed','cancelled')
 OR NEW.run_id!=OLD.run_id OR NEW.agent_id!=OLD.agent_id OR NEW.kind!=OLD.kind OR NEW.model_snapshot_id!=OLD.model_snapshot_id
 OR NEW.capability_snapshot_id!=OLD.capability_snapshot_id OR NEW.created_at!=OLD.created_at OR NEW.history_boundary!=OLD.history_boundary
 OR NEW.conversation_id!=OLD.conversation_id OR NEW.task_id IS NOT OLD.task_id OR NEW.parent_run_id IS NOT OLD.parent_run_id
 OR NEW.root_run_id IS NOT OLD.root_run_id OR NEW.worker_position IS NOT OLD.worker_position OR NEW.depth!=OLD.depth OR NEW.task_epoch IS NOT OLD.task_epoch
 OR NOT ((NEW.status=OLD.status AND NEW.desired_state!='running' AND NEW.checkpoint_json=OLD.checkpoint_json) OR (OLD.status='queued' AND NEW.status IN ('running','cancelled')) OR (OLD.status='running' AND NEW.status IN ('running','waiting','completed','failed','cancelled')) OR (OLD.status='waiting' AND NEW.status IN ('running','cancelled')))
BEGIN SELECT RAISE(ABORT,'invalid run transition'); END;
