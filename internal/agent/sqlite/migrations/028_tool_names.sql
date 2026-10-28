-- Preserve historical names and hashes. Only execution guards learn the new
-- builtin wire spelling; the old Code Mode spelling remains cleanup-only.
DROP TRIGGER IF EXISTS child_initial;
CREATE TRIGGER child_initial BEFORE INSERT ON run WHEN NEW.worker_position='child' AND NOT EXISTS(
 SELECT 1 FROM run p JOIN capability_snapshot pc ON pc.snapshot_id=p.capability_snapshot_id
 JOIN capability_snapshot cc ON cc.snapshot_id=NEW.capability_snapshot_id
 WHERE p.run_id=NEW.parent_run_id AND p.kind='worker' AND p.status='running' AND p.desired_state='running'
 AND p.task_id=NEW.task_id AND p.root_run_id=NEW.root_run_id AND p.task_epoch=NEW.task_epoch
 AND p.agent_id=NEW.agent_id AND p.conversation_id=NEW.conversation_id AND NEW.depth=p.depth+1
 AND p.model_snapshot_id=NEW.model_snapshot_id AND pc.can_spawn_agents=1
 AND cc.role='worker' AND cc.data_scope=pc.data_scope AND cc.agent_config_version=pc.agent_config_version
 AND cc.can_spawn_agents<=pc.can_spawn_agents AND cc.format_version=pc.format_version
 AND NOT EXISTS(SELECT 1 FROM json_each(cc.tools_json) child WHERE NOT EXISTS(SELECT 1 FROM json_each(pc.tools_json) parent WHERE child.value=parent.value))
 AND NOT EXISTS(SELECT 1 FROM json_each(cc.tools_json) child WHERE json_extract(child.value,'$.permission') NOT IN ('read_only','external_effect','artifact_write','spawn_agents') OR json_extract(child.value,'$.name') GLOB 'builtin__task_*')
) BEGIN SELECT RAISE(ABORT,'invalid child inheritance'); END;

DROP TRIGGER IF EXISTS code_wrapper_terminal;
CREATE TRIGGER code_wrapper_terminal AFTER UPDATE OF status ON tool_invocation WHEN NEW.tool_name IN ('builtin__codemode__execute','builtin__codemode_execute') AND NEW.status IN ('succeeded','failed','cancelled','unknown') BEGIN
 UPDATE code_execution_state SET state=CASE WHEN NEW.status='succeeded' THEN 'completed' WHEN NEW.status='cancelled' THEN 'cancelled' ELSE 'failed' END,processed_wake_seq=wake_seq,result_ref=NEW.result_ref,error_json=NEW.error_json,claim_token=NULL,owner_run_token=NULL,claim_expires_at=0,updated_at=NEW.updated_at,version=version+1 WHERE execution_id=NEW.invocation_id;
END;
