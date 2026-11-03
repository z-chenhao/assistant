CREATE TABLE task_feedback (
 feedback_ref TEXT PRIMARY KEY,
 task_id TEXT NOT NULL REFERENCES task(task_id),
 task_epoch INTEGER NOT NULL CHECK(task_epoch>=1),
 privacy_epoch INTEGER NOT NULL CHECK(privacy_epoch>=0),
 kind TEXT NOT NULL CHECK(kind IN ('input','review')),
 prompt TEXT NOT NULL,
 source_message_id TEXT NOT NULL,
 artifact_refs TEXT NOT NULL CHECK(json_valid(artifact_refs)),
 status TEXT NOT NULL CHECK(status IN ('pending','superseded','answered')),
 created_at INTEGER NOT NULL,
 expires_at INTEGER NOT NULL CHECK(expires_at>created_at),
 response_message_id TEXT REFERENCES main_input(message_id),
 CHECK((status='answered')=(response_message_id IS NOT NULL))
) STRICT;
CREATE UNIQUE INDEX task_feedback_pending ON task_feedback(task_id) WHERE status='pending';
CREATE INDEX task_feedback_task ON task_feedback(task_id);
CREATE UNIQUE INDEX task_feedback_response ON task_feedback(response_message_id) WHERE response_message_id IS NOT NULL;

CREATE TRIGGER task_feedback_identity BEFORE UPDATE ON task_feedback WHEN
 NEW.rowid!=OLD.rowid OR NEW.feedback_ref!=OLD.feedback_ref OR NEW.task_id!=OLD.task_id OR
 NEW.task_epoch!=OLD.task_epoch OR NEW.privacy_epoch!=OLD.privacy_epoch OR
 NEW.kind!=OLD.kind OR NEW.prompt!=OLD.prompt OR NEW.source_message_id!=OLD.source_message_id OR
 NEW.artifact_refs!=OLD.artifact_refs OR NEW.created_at!=OLD.created_at OR NEW.expires_at!=OLD.expires_at OR
 OLD.status!='pending' OR NEW.status NOT IN ('superseded','answered')
 BEGIN SELECT RAISE(ABORT,'immutable task feedback'); END;
CREATE TRIGGER task_feedback_no_delete BEFORE DELETE ON task_feedback
 BEGIN SELECT RAISE(ABORT,'task feedback audit retained'); END;
CREATE TRIGGER task_feedback_no_replace BEFORE INSERT ON task_feedback WHEN
 EXISTS(SELECT 1 FROM task_feedback WHERE feedback_ref=NEW.feedback_ref OR
 (task_id=NEW.task_id AND status='pending' AND NEW.status='pending'))
 BEGIN SELECT RAISE(ABORT,'task feedback identity exists'); END;
