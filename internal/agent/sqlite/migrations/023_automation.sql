CREATE TABLE automation (
 automation_id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, instruction TEXT NOT NULL,
 trigger_type TEXT NOT NULL CHECK(trigger_type IN ('at','cron')), trigger_spec TEXT NOT NULL, timezone TEXT NOT NULL,
 enabled INTEGER NOT NULL CHECK(enabled IN (0,1)), next_fire_at INTEGER,
 created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, version INTEGER NOT NULL CHECK(version>=1),
 schedule_version INTEGER NOT NULL DEFAULT 1, task_id TEXT UNIQUE REFERENCES task(task_id), below_value INTEGER, is_below INTEGER NOT NULL DEFAULT 0 CHECK(is_below IN (0,1)),
 last_observed_fire INTEGER NOT NULL DEFAULT 0
) STRICT;
CREATE INDEX automation_due ON automation(next_fire_at) WHERE enabled=1 AND next_fire_at IS NOT NULL;
CREATE TRIGGER automation_version BEFORE UPDATE ON automation WHEN NEW.version!=OLD.version+1 OR NEW.automation_id!=OLD.automation_id OR NEW.agent_id!=OLD.agent_id OR NEW.created_at!=OLD.created_at
BEGIN SELECT RAISE(ABORT,'invalid automation version'); END;
CREATE TABLE automation_fire (
 event_id TEXT PRIMARY KEY, automation_id TEXT NOT NULL REFERENCES automation(automation_id), schedule_version INTEGER NOT NULL,
 fire_at INTEGER NOT NULL, instruction TEXT NOT NULL, below_value INTEGER, state TEXT NOT NULL CHECK(state IN ('pending','admitted','skipped')),
 root_run_id TEXT UNIQUE REFERENCES run(run_id), observed_value INTEGER, notify INTEGER NOT NULL DEFAULT 0 CHECK(notify IN (0,1)),
 UNIQUE(automation_id,schedule_version,fire_at)
) STRICT;
CREATE INDEX automation_pending ON automation_fire(fire_at) WHERE state='pending';
CREATE TABLE automation_request (
 agent_id TEXT NOT NULL, request_key TEXT NOT NULL, request_hash TEXT NOT NULL, result_json TEXT NOT NULL CHECK(json_valid(result_json)),
 PRIMARY KEY(agent_id,request_key)
) STRICT;
CREATE TRIGGER automation_request_immutable BEFORE UPDATE ON automation_request BEGIN SELECT RAISE(ABORT,'immutable automation receipt'); END;
CREATE TRIGGER automation_no_delete BEFORE DELETE ON automation BEGIN SELECT RAISE(ABORT,'automation audit retained'); END;
CREATE TRIGGER automation_fire_transition BEFORE UPDATE ON automation_fire WHEN
 NEW.event_id!=OLD.event_id OR NEW.automation_id!=OLD.automation_id OR NEW.schedule_version!=OLD.schedule_version
 OR NEW.fire_at!=OLD.fire_at OR NEW.instruction!=OLD.instruction OR NEW.below_value IS NOT OLD.below_value
 OR (OLD.state!='pending' AND NEW.state!=OLD.state) OR (OLD.root_run_id IS NOT NULL AND NEW.root_run_id IS NOT OLD.root_run_id)
 OR (OLD.observed_value IS NOT NULL AND (NEW.observed_value IS NOT OLD.observed_value OR NEW.notify!=OLD.notify))
 BEGIN SELECT RAISE(ABORT,'invalid automation fire transition'); END;
CREATE TRIGGER automation_fire_no_delete BEFORE DELETE ON automation_fire BEGIN SELECT RAISE(ABORT,'automation fire audit retained'); END;
