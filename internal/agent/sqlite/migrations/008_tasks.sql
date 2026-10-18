CREATE TABLE task (
 task_id TEXT PRIMARY KEY, task_ref TEXT NOT NULL UNIQUE, agent_id TEXT NOT NULL, conversation_id TEXT NOT NULL,
 title TEXT NOT NULL, anchor_task TEXT NOT NULL, anchor_artifact_refs TEXT NOT NULL CHECK(json_valid(anchor_artifact_refs)),
 status TEXT NOT NULL CHECK(status IN ('active','paused','completed','cancelled')), task_epoch INTEGER NOT NULL CHECK(task_epoch>=1),
 progress_summary TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
 completed_at INTEGER, cancelled_at INTEGER, version INTEGER NOT NULL CHECK(version>=1),
 CHECK((status='completed')=(completed_at IS NOT NULL)), CHECK((status='cancelled')=(cancelled_at IS NOT NULL))
) STRICT;
CREATE INDEX task_agent_status ON task(agent_id,status,updated_at);
CREATE TRIGGER task_version BEFORE UPDATE ON task WHEN NEW.version!=OLD.version+1 OR NEW.task_id!=OLD.task_id OR NEW.task_ref!=OLD.task_ref
 OR NEW.agent_id!=OLD.agent_id OR NEW.conversation_id!=OLD.conversation_id OR NEW.title!=OLD.title OR NEW.anchor_task!=OLD.anchor_task
 OR NEW.anchor_artifact_refs!=OLD.anchor_artifact_refs OR NEW.created_at!=OLD.created_at OR NEW.task_epoch<OLD.task_epoch
 OR NEW.task_epoch>OLD.task_epoch+1 OR (NEW.task_epoch!=OLD.task_epoch AND NEW.status!='active')
 OR (NEW.status!=OLD.status AND NOT ((OLD.status='active' AND NEW.status IN ('paused','completed','cancelled')) OR (OLD.status IN ('paused','completed','cancelled') AND NEW.status='active')))
BEGIN SELECT RAISE(ABORT,'invalid task transition'); END;
CREATE TRIGGER task_no_delete BEFORE DELETE ON task BEGIN SELECT RAISE(ABORT,'task audit retained'); END;
CREATE TRIGGER task_no_replace BEFORE INSERT ON task WHEN EXISTS(SELECT 1 FROM task WHERE task_id=NEW.task_id) BEGIN SELECT RAISE(ABORT,'task identity exists'); END;
CREATE TRIGGER task_initial BEFORE INSERT ON task WHEN NEW.version!=1 OR NEW.status!='active' OR NEW.task_epoch!=1 BEGIN SELECT RAISE(ABORT,'task starts active at epoch/version one'); END;
CREATE TRIGGER worker_task_initial BEFORE INSERT ON run WHEN NEW.kind='worker' AND NOT EXISTS(SELECT 1 FROM task t WHERE t.task_id=NEW.task_id AND t.agent_id=NEW.agent_id AND t.conversation_id=NEW.conversation_id AND t.task_epoch=NEW.task_epoch AND t.status='active') BEGIN SELECT RAISE(ABORT,'invalid worker task scope'); END;
CREATE TABLE task_instruction (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT, instruction_id TEXT NOT NULL UNIQUE, task_id TEXT NOT NULL REFERENCES task(task_id),
 task_epoch INTEGER NOT NULL, source_message_id TEXT, instruction TEXT NOT NULL, artifact_refs TEXT NOT NULL CHECK(json_valid(artifact_refs)), created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX task_instruction_epoch ON task_instruction(task_id,task_epoch,sequence);
CREATE TRIGGER instruction_no_update BEFORE UPDATE ON task_instruction BEGIN SELECT RAISE(ABORT,'immutable task instruction'); END;
CREATE TRIGGER instruction_no_delete BEFORE DELETE ON task_instruction BEGIN SELECT RAISE(ABORT,'instruction audit retained'); END;
-- Runtime binds the user input boundary plus canonical action, independent of
-- provider call IDs. Distinct actions in one source remain distinct requests.
CREATE TABLE task_request (
 source_message_id TEXT NOT NULL, request_hash TEXT NOT NULL, receipt_json TEXT NOT NULL CHECK(json_valid(receipt_json)),
 invocation_id TEXT NOT NULL UNIQUE REFERENCES tool_invocation(invocation_id), PRIMARY KEY(source_message_id,request_hash)
) STRICT;
CREATE TABLE task_invocation (
 invocation_id TEXT PRIMARY KEY REFERENCES tool_invocation(invocation_id), request_hash TEXT NOT NULL, receipt_json TEXT NOT NULL CHECK(json_valid(receipt_json))
) STRICT;
CREATE TRIGGER task_request_no_update BEFORE UPDATE ON task_request BEGIN SELECT RAISE(ABORT,'immutable task request'); END;
CREATE TRIGGER task_invocation_no_update BEFORE UPDATE ON task_invocation BEGIN SELECT RAISE(ABORT,'immutable task invocation'); END;
CREATE TABLE worker_result (
 run_id TEXT NOT NULL REFERENCES run(run_id), exchange_id TEXT PRIMARY KEY REFERENCES model_generation(generation_id),
 content TEXT NOT NULL, created_at INTEGER NOT NULL
) STRICT;
CREATE TRIGGER result_no_update BEFORE UPDATE ON worker_result BEGIN SELECT RAISE(ABORT,'immutable worker result'); END;
CREATE TABLE task_update (
 event_id TEXT PRIMARY KEY, task_id TEXT NOT NULL REFERENCES task(task_id), root_run_id TEXT NOT NULL REFERENCES run(run_id),
 task_epoch INTEGER NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('completed','failed','needs_input','unknown')),
 content TEXT NOT NULL, artifact_refs TEXT NOT NULL CHECK(json_valid(artifact_refs)), occurred_at INTEGER NOT NULL,
 UNIQUE(root_run_id,kind)
) STRICT;
CREATE TRIGGER task_update_no_update BEFORE UPDATE ON task_update BEGIN SELECT RAISE(ABORT,'immutable task update'); END;
CREATE TRIGGER task_update_no_delete BEFORE DELETE ON task_update BEGIN SELECT RAISE(ABORT,'task update audit retained'); END;
CREATE TABLE main_inbox (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT, event_id TEXT NOT NULL UNIQUE REFERENCES task_update(event_id),
 request_id TEXT NOT NULL UNIQUE, agent_id TEXT NOT NULL, run_id TEXT REFERENCES run(run_id), message_json TEXT NOT NULL CHECK(json_valid(message_json)), created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX inbox_pending ON main_inbox(agent_id,sequence) WHERE run_id IS NULL;
CREATE TRIGGER inbox_identity BEFORE UPDATE ON main_inbox WHEN NEW.event_id!=OLD.event_id OR NEW.request_id!=OLD.request_id OR NEW.message_json!=OLD.message_json OR (OLD.run_id IS NOT NULL AND (NEW.run_id IS NOT NULL OR EXISTS(SELECT 1 FROM model_generation g WHERE g.run_id=OLD.run_id))) BEGIN SELECT RAISE(ABORT,'immutable inbox assignment'); END;
CREATE TRIGGER instruction_no_replace BEFORE INSERT ON task_instruction WHEN EXISTS(SELECT 1 FROM task_instruction WHERE instruction_id=NEW.instruction_id OR sequence=NEW.sequence) BEGIN SELECT RAISE(ABORT,'instruction identity exists'); END;
CREATE TRIGGER task_request_no_delete BEFORE DELETE ON task_request BEGIN SELECT RAISE(ABORT,'task request audit retained'); END;
CREATE TRIGGER task_request_no_replace BEFORE INSERT ON task_request WHEN EXISTS(SELECT 1 FROM task_request WHERE (source_message_id=NEW.source_message_id AND request_hash=NEW.request_hash) OR invocation_id=NEW.invocation_id) BEGIN SELECT RAISE(ABORT,'task request identity exists'); END;
CREATE TRIGGER task_invocation_no_delete BEFORE DELETE ON task_invocation BEGIN SELECT RAISE(ABORT,'task receipt audit retained'); END;
CREATE TRIGGER task_invocation_no_replace BEFORE INSERT ON task_invocation WHEN EXISTS(SELECT 1 FROM task_invocation WHERE invocation_id=NEW.invocation_id) BEGIN SELECT RAISE(ABORT,'task receipt identity exists'); END;
CREATE TRIGGER result_no_delete BEFORE DELETE ON worker_result BEGIN SELECT RAISE(ABORT,'worker result audit retained'); END;
CREATE TRIGGER result_no_replace BEFORE INSERT ON worker_result WHEN EXISTS(SELECT 1 FROM worker_result WHERE exchange_id=NEW.exchange_id) AND (NEW.run_id!=(SELECT run_id FROM worker_result WHERE exchange_id=NEW.exchange_id) OR NEW.content!=(SELECT content FROM worker_result WHERE exchange_id=NEW.exchange_id)) BEGIN SELECT RAISE(ABORT,'worker result identity conflict'); END;
CREATE TRIGGER task_update_no_replace BEFORE INSERT ON task_update WHEN EXISTS(SELECT 1 FROM task_update WHERE event_id=NEW.event_id OR (root_run_id=NEW.root_run_id AND kind=NEW.kind)) BEGIN SELECT RAISE(ABORT,'task update identity exists'); END;
CREATE TRIGGER inbox_no_delete BEFORE DELETE ON main_inbox BEGIN SELECT RAISE(ABORT,'main inbox audit retained'); END;
