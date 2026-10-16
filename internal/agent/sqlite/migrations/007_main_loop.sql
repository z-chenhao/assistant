CREATE TABLE capability_snapshot (
 snapshot_id TEXT PRIMARY KEY, agent_config_version INTEGER NOT NULL, role TEXT NOT NULL CHECK(role IN ('main','worker')),
 tool_specs_hash TEXT NOT NULL, skill_set_hash TEXT NOT NULL, data_scope TEXT NOT NULL,
 can_spawn_agents INTEGER NOT NULL CHECK(can_spawn_agents IN (0,1)), tools_json TEXT NOT NULL CHECK(json_valid(tools_json)), created_at INTEGER NOT NULL
) STRICT;
CREATE TRIGGER capability_no_update BEFORE UPDATE ON capability_snapshot BEGIN SELECT RAISE(ABORT,'immutable capability'); END;
CREATE TRIGGER capability_no_delete BEFORE DELETE ON capability_snapshot BEGIN SELECT RAISE(ABORT,'immutable capability'); END;
CREATE TRIGGER capability_no_replace BEFORE INSERT ON capability_snapshot WHEN EXISTS(SELECT 1 FROM capability_snapshot WHERE snapshot_id=NEW.snapshot_id) BEGIN SELECT RAISE(ABORT,'immutable capability'); END;

CREATE TABLE run (
 run_id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, conversation_id TEXT NOT NULL,
 task_id TEXT, parent_run_id TEXT REFERENCES run(run_id), root_run_id TEXT REFERENCES run(run_id),
 kind TEXT NOT NULL CHECK(kind IN ('main','worker')), worker_position TEXT CHECK(worker_position IN ('root','child')),
 status TEXT NOT NULL CHECK(status IN ('queued','running','waiting','completed','failed','cancelled')),
 wait_reason TEXT CHECK(wait_reason IN ('user_input','approval','external_event','resource')),
 task_epoch INTEGER, depth INTEGER NOT NULL DEFAULT 0,
 capability_snapshot_id TEXT NOT NULL REFERENCES capability_snapshot(snapshot_id), model_snapshot_id TEXT NOT NULL REFERENCES model_snapshot(snapshot_id),
 desired_state TEXT NOT NULL DEFAULT 'running', checkpoint_ref TEXT, result_ref TEXT, error_code TEXT,
 checkpoint_json TEXT NOT NULL CHECK(json_valid(checkpoint_json)), history_boundary INTEGER NOT NULL,
 started_at INTEGER, ended_at INTEGER, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, version INTEGER NOT NULL CHECK(version>=1),
 CHECK((kind='main' AND task_id IS NULL AND parent_run_id IS NULL AND worker_position IS NULL AND depth=0) OR
       (kind='worker' AND task_id IS NOT NULL AND ((worker_position='root' AND parent_run_id IS NULL) OR (worker_position='child' AND parent_run_id IS NOT NULL)))),
 CHECK((status='waiting')=(wait_reason IS NOT NULL)),
 CHECK((status IN ('completed','failed','cancelled'))=(ended_at IS NOT NULL))
) STRICT;
CREATE UNIQUE INDEX one_main ON run(agent_id) WHERE kind='main' AND status IN ('queued','running','waiting');
CREATE UNIQUE INDEX one_root ON run(task_id) WHERE kind='worker' AND worker_position='root' AND status IN ('queued','running','waiting');
CREATE INDEX run_parent_status ON run(parent_run_id,status);
CREATE INDEX run_task_status ON run(task_id,status);
CREATE INDEX run_recovery ON run(status,wait_reason);
CREATE TRIGGER run_version BEFORE UPDATE ON run WHEN NEW.version!=OLD.version+1 OR OLD.status IN ('completed','failed','cancelled')
 OR NEW.run_id!=OLD.run_id OR NEW.agent_id!=OLD.agent_id OR NEW.kind!=OLD.kind OR NEW.model_snapshot_id!=OLD.model_snapshot_id
 OR NEW.capability_snapshot_id!=OLD.capability_snapshot_id OR NEW.created_at!=OLD.created_at OR NEW.history_boundary!=OLD.history_boundary
 OR NEW.conversation_id!=OLD.conversation_id OR NEW.task_id IS NOT OLD.task_id OR NEW.parent_run_id IS NOT OLD.parent_run_id
 OR NEW.root_run_id IS NOT OLD.root_run_id OR NEW.worker_position IS NOT OLD.worker_position OR NEW.depth!=OLD.depth OR NEW.task_epoch IS NOT OLD.task_epoch
 OR NOT ((OLD.status='queued' AND NEW.status IN ('running','cancelled')) OR (OLD.status='running' AND NEW.status IN ('running','waiting','completed','failed','cancelled')) OR (OLD.status='waiting' AND NEW.status IN ('running','cancelled')))
BEGIN SELECT RAISE(ABORT,'invalid run transition'); END;
CREATE TRIGGER run_no_delete BEFORE DELETE ON run BEGIN SELECT RAISE(ABORT,'run audit retained'); END;
CREATE TRIGGER run_no_replace BEFORE INSERT ON run WHEN EXISTS(SELECT 1 FROM run WHERE run_id=NEW.run_id) BEGIN SELECT RAISE(ABORT,'run identity exists'); END;
CREATE TRIGGER run_initial BEFORE INSERT ON run WHEN NEW.version!=1 OR NEW.status!='queued' BEGIN SELECT RAISE(ABORT,'run starts queued at version one'); END;

CREATE TABLE run_claim (
 run_id TEXT PRIMARY KEY REFERENCES run(run_id), epoch INTEGER NOT NULL, token TEXT, expires_at INTEGER NOT NULL, version INTEGER NOT NULL
) STRICT;
CREATE INDEX run_claim_expiry ON run_claim(expires_at);

CREATE TABLE main_input (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT, agent_id TEXT NOT NULL, request_key TEXT NOT NULL,
 run_id TEXT NOT NULL REFERENCES run(run_id), message_id TEXT NOT NULL UNIQUE, message_json TEXT NOT NULL CHECK(json_valid(message_json)),
 UNIQUE(agent_id,request_key)
) STRICT;
CREATE INDEX main_input_run_sequence ON main_input(run_id,sequence);
CREATE TRIGGER input_no_update BEFORE UPDATE ON main_input BEGIN SELECT RAISE(ABORT,'immutable input'); END;
CREATE TRIGGER input_no_delete BEFORE DELETE ON main_input BEGIN SELECT RAISE(ABORT,'input audit retained'); END;

-- Cross-database write intents are immutable. Replaying their ordered prefix
-- cannot produce a different semantic message, even after ownership changes.
CREATE TABLE conversation_intent (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT, message_id TEXT NOT NULL UNIQUE, message_json TEXT NOT NULL CHECK(json_valid(message_json)), flushed INTEGER NOT NULL DEFAULT 0
) STRICT;
CREATE TRIGGER conversation_intent_identity BEFORE UPDATE ON conversation_intent WHEN NEW.sequence!=OLD.sequence OR NEW.message_id!=OLD.message_id OR NEW.message_json!=OLD.message_json OR NEW.flushed<OLD.flushed BEGIN SELECT RAISE(ABORT,'immutable message intent'); END;
CREATE TRIGGER intent_no_delete BEFORE DELETE ON conversation_intent BEGIN SELECT RAISE(ABORT,'message audit retained'); END;

CREATE TABLE model_generation (
 generation_id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES run(run_id), step_index INTEGER NOT NULL,
 request_json TEXT NOT NULL CHECK(json_valid(request_json)), request_hash TEXT NOT NULL, owner_token TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','completed','failed')), result_json TEXT, error_code TEXT NOT NULL DEFAULT '',
 attempt_count INTEGER NOT NULL DEFAULT 1, created_at INTEGER NOT NULL, completed_at INTEGER, UNIQUE(run_id,step_index)
) STRICT;
CREATE TRIGGER generation_frozen BEFORE UPDATE ON model_generation WHEN OLD.status!='pending' OR NEW.request_json!=OLD.request_json OR NEW.request_hash!=OLD.request_hash OR NEW.generation_id!=OLD.generation_id OR NEW.run_id!=OLD.run_id OR NEW.step_index!=OLD.step_index BEGIN SELECT RAISE(ABORT,'immutable model exchange'); END;
CREATE TRIGGER generation_no_delete BEFORE DELETE ON model_generation BEGIN SELECT RAISE(ABORT,'generation audit retained'); END;
CREATE TRIGGER generation_no_replace BEFORE INSERT ON model_generation WHEN EXISTS(SELECT 1 FROM model_generation WHERE generation_id=NEW.generation_id) BEGIN SELECT RAISE(ABORT,'immutable model exchange'); END;

CREATE TABLE tool_invocation (
 invocation_id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES run(run_id), parent_invocation_id TEXT REFERENCES tool_invocation(invocation_id),
 provider_exchange_id TEXT NOT NULL REFERENCES model_generation(generation_id), provider_call_id TEXT NOT NULL,
 tool_name TEXT NOT NULL, arguments_json TEXT NOT NULL, request_hash TEXT NOT NULL, replay_policy TEXT NOT NULL CHECK(replay_policy IN ('safe','query_before_retry','never')),
 idempotency_key TEXT NOT NULL UNIQUE, status TEXT NOT NULL CHECK(status IN ('queued','running','waiting','succeeded','failed','cancelled','unknown')),
 wait_reason TEXT, attempt_count INTEGER NOT NULL DEFAULT 0, result_ref TEXT, error_json TEXT, result_json TEXT,
 result_ready_at INTEGER, delivery_seq INTEGER NOT NULL, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, version INTEGER NOT NULL,
 UNIQUE(run_id,provider_exchange_id,provider_call_id)
) STRICT;
CREATE TRIGGER invocation_transition BEFORE UPDATE ON tool_invocation WHEN NEW.version!=OLD.version+1 OR OLD.status IN ('succeeded','failed','cancelled') OR NEW.arguments_json!=OLD.arguments_json OR NEW.request_hash!=OLD.request_hash OR NEW.replay_policy!=OLD.replay_policy OR NEW.idempotency_key!=OLD.idempotency_key OR NEW.provider_call_id!=OLD.provider_call_id OR NEW.provider_exchange_id!=OLD.provider_exchange_id OR NEW.tool_name!=OLD.tool_name OR NOT ((OLD.status='queued' AND NEW.status IN ('running','cancelled')) OR (OLD.status='running' AND NEW.status IN ('running','succeeded','failed','cancelled')))
BEGIN SELECT RAISE(ABORT,'invalid tool transition'); END;
CREATE TRIGGER invocation_no_delete BEFORE DELETE ON tool_invocation BEGIN SELECT RAISE(ABORT,'tool audit retained'); END;
CREATE TRIGGER invocation_no_replace BEFORE INSERT ON tool_invocation WHEN EXISTS(SELECT 1 FROM tool_invocation WHERE invocation_id=NEW.invocation_id) BEGIN SELECT RAISE(ABORT,'tool identity exists'); END;

CREATE TABLE main_reply (
 exchange_id TEXT PRIMARY KEY REFERENCES model_generation(generation_id), run_id TEXT NOT NULL REFERENCES run(run_id), message_id TEXT NOT NULL UNIQUE, delivery_id TEXT NOT NULL UNIQUE
) STRICT;
CREATE TABLE delivery_request (
 delivery_id TEXT PRIMARY KEY, message_id TEXT NOT NULL UNIQUE, channel TEXT NOT NULL, destination_ref TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','sent','failed')), idempotency_key TEXT NOT NULL UNIQUE, attempt_count INTEGER NOT NULL DEFAULT 0,
 last_error TEXT, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
) STRICT;
CREATE TABLE run_trace (
 run_id TEXT NOT NULL REFERENCES run(run_id), sequence INTEGER NOT NULL, event_json TEXT NOT NULL CHECK(json_valid(event_json)), occurred_at INTEGER NOT NULL,
 PRIMARY KEY(run_id,sequence)
) STRICT;
