-- A manual complete is an explicit stop; ordinary once-Root completion is not.
ALTER TABLE task ADD COLUMN explicit_end_epoch INTEGER NOT NULL DEFAULT 0 CHECK(explicit_end_epoch>=0);
DROP TRIGGER run_desired_state;
CREATE TRIGGER run_desired_state BEFORE UPDATE ON run WHEN NEW.desired_state NOT IN
 ('running','task_paused','task_cancelled','memory_forgotten','github_revoked','github_user_superseded')
 OR (OLD.desired_state!='running' AND NEW.desired_state='running')
 BEGIN SELECT RAISE(ABORT,'invalid desired state'); END;
CREATE TABLE github_task_binding (
 scope TEXT PRIMARY KEY REFERENCES github_watch(scope), task_id TEXT NOT NULL REFERENCES task(task_id),
 task_epoch INTEGER NOT NULL, privacy_epoch INTEGER NOT NULL, enabled INTEGER NOT NULL CHECK(enabled IN(0,1)),
 generation INTEGER NOT NULL CHECK(generation>=1), last_sequence INTEGER NOT NULL,
 version INTEGER NOT NULL CHECK(version>=1)
) STRICT;
CREATE TRIGGER github_task_binding_version BEFORE UPDATE ON github_task_binding WHEN
 NEW.version!=OLD.version+1 OR NEW.scope!=OLD.scope OR NEW.generation<OLD.generation
 BEGIN SELECT RAISE(ABORT,'invalid GitHub task binding'); END;
CREATE TRIGGER github_task_binding_retained BEFORE DELETE ON github_task_binding BEGIN SELECT RAISE(ABORT,'binding retained'); END;
CREATE TABLE github_task_followup (
 run_id TEXT PRIMARY KEY REFERENCES run(run_id), request_id TEXT NOT NULL UNIQUE,
 observation_sequence INTEGER NOT NULL UNIQUE REFERENCES github_observation(sequence),
 scope TEXT NOT NULL REFERENCES github_task_binding(scope), generation INTEGER NOT NULL,
 task_id TEXT NOT NULL REFERENCES task(task_id), task_epoch INTEGER NOT NULL, privacy_epoch INTEGER NOT NULL,
 state TEXT NOT NULL CHECK(state IN('assigned','continued','ignored')),
 continued_epoch INTEGER, created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX github_task_followup_scope ON github_task_followup(scope,state);
CREATE TRIGGER github_task_followup_identity BEFORE UPDATE ON github_task_followup WHEN
 NEW.run_id!=OLD.run_id OR NEW.request_id!=OLD.request_id OR NEW.observation_sequence!=OLD.observation_sequence OR NEW.scope!=OLD.scope OR
 NEW.generation!=OLD.generation OR NEW.task_id!=OLD.task_id OR NEW.task_epoch!=OLD.task_epoch OR NEW.privacy_epoch!=OLD.privacy_epoch OR NEW.created_at!=OLD.created_at OR
 OLD.state='ignored' OR (OLD.state='continued' AND NEW.state!='ignored')
 BEGIN SELECT RAISE(ABORT,'immutable GitHub followup identity'); END;
CREATE TRIGGER github_task_followup_retained BEFORE DELETE ON github_task_followup BEGIN SELECT RAISE(ABORT,'followup retained'); END;
