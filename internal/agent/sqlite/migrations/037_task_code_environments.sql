CREATE TABLE task_code_environment (
 task_id TEXT PRIMARY KEY REFERENCES task(task_id), scope_hash TEXT NOT NULL, source_path TEXT NOT NULL,
 directory TEXT NOT NULL, baseline_path TEXT NOT NULL, baseline_hash TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL CHECK(status IN ('pending','ready','failed')), error_code TEXT NOT NULL DEFAULT ''
) STRICT;
CREATE TRIGGER task_code_environment_identity BEFORE UPDATE ON task_code_environment
 WHEN NEW.task_id!=OLD.task_id OR NEW.scope_hash!=OLD.scope_hash OR NEW.source_path!=OLD.source_path
 OR NEW.directory!=OLD.directory OR NEW.baseline_path!=OLD.baseline_path
 OR (OLD.status='ready' AND (NEW.status!='ready' OR NEW.baseline_hash!=OLD.baseline_hash))
 BEGIN SELECT RAISE(ABORT,'immutable code workspace'); END;
CREATE TRIGGER task_code_environment_retained BEFORE DELETE ON task_code_environment BEGIN SELECT RAISE(ABORT,'code workspace retained'); END;
