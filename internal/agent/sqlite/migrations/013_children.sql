-- Root-level safety counters are durable and change only on first admission.
-- Old frozen capabilities remain untouched; only new Roots gain delegation.
CREATE TABLE run_tree (
 root_run_id TEXT PRIMARY KEY REFERENCES run(run_id),
 descendants INTEGER NOT NULL DEFAULT 0 CHECK(descendants>=0),
 tool_calls INTEGER NOT NULL DEFAULT 0 CHECK(tool_calls>=0),
 max_depth INTEGER NOT NULL CHECK(max_depth BETWEEN 1 AND 16),
 max_descendants INTEGER NOT NULL CHECK(max_descendants BETWEEN 1 AND 10000),
 max_tool_calls INTEGER NOT NULL CHECK(max_tool_calls BETWEEN 1 AND 50000),
 version INTEGER NOT NULL CHECK(version>=1)
) STRICT;
INSERT INTO run_tree SELECT r.run_id,(SELECT count(*) FROM run c WHERE c.root_run_id=r.run_id AND c.worker_position='child'),
 (SELECT count(*) FROM tool_invocation i JOIN run c USING(run_id) WHERE c.root_run_id=r.run_id),8,4096,10000,1
 FROM run r WHERE r.worker_position='root';
CREATE TRIGGER tree_version BEFORE UPDATE ON run_tree WHEN NEW.version!=OLD.version+1 OR NEW.root_run_id!=OLD.root_run_id
 OR NEW.max_depth!=OLD.max_depth OR NEW.max_descendants!=OLD.max_descendants OR NEW.max_tool_calls!=OLD.max_tool_calls
 OR NEW.descendants<OLD.descendants OR NEW.descendants>OLD.descendants+1 OR NEW.tool_calls<OLD.tool_calls OR NEW.tool_calls>OLD.tool_calls+1
BEGIN SELECT RAISE(ABORT,'invalid tree counters'); END;
CREATE TRIGGER tree_no_delete BEFORE DELETE ON run_tree BEGIN SELECT RAISE(ABORT,'tree audit retained'); END;
CREATE TRIGGER tree_no_replace BEFORE INSERT ON run_tree WHEN EXISTS(SELECT 1 FROM run_tree WHERE root_run_id=NEW.root_run_id) BEGIN SELECT RAISE(ABORT,'tree identity exists'); END;
CREATE TABLE agent_invocation (
 invocation_id TEXT PRIMARY KEY REFERENCES tool_invocation(invocation_id),
 child_run_id TEXT NOT NULL UNIQUE REFERENCES run(run_id),
 request_hash TEXT NOT NULL
) STRICT;
CREATE TRIGGER agent_invocation_no_update BEFORE UPDATE ON agent_invocation BEGIN SELECT RAISE(ABORT,'immutable child admission'); END;
CREATE TRIGGER agent_invocation_no_delete BEFORE DELETE ON agent_invocation BEGIN SELECT RAISE(ABORT,'child audit retained'); END;
CREATE TRIGGER agent_invocation_no_replace BEFORE INSERT ON agent_invocation WHEN EXISTS(SELECT 1 FROM agent_invocation WHERE invocation_id=NEW.invocation_id OR child_run_id=NEW.child_run_id) BEGIN SELECT RAISE(ABORT,'child admission exists'); END;
-- Enforce immutable tree ancestry and frozen model identity at the storage edge.
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
 AND NOT EXISTS(SELECT 1 FROM json_each(cc.tools_json) child WHERE json_extract(child.value,'$.permission') NOT IN ('read_only','external_effect','artifact_write','spawn_agents') OR json_extract(child.value,'$.name') LIKE 'builtin__task__%')
) BEGIN SELECT RAISE(ABORT,'invalid child inheritance'); END;
