-- One logical Computer for this personal agent. Instances are disposable;
-- ownership belongs to a fenced Code Mode invocation, never JavaScript input.
CREATE TABLE computer (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 computer_id TEXT NOT NULL UNIQUE,
 instance_epoch INTEGER NOT NULL CHECK(instance_epoch>=1),
 instance_json TEXT NOT NULL CHECK(json_valid(instance_json)),
 compatibility_json TEXT NOT NULL CHECK(json_valid(compatibility_json)),
 manifest_json TEXT NOT NULL CHECK(json_valid(manifest_json)),
 checkpoint_json TEXT CHECK(checkpoint_json IS NULL OR json_valid(checkpoint_json)),
 phase TEXT NOT NULL CHECK(phase IN ('restoring','ready','checkpointing','checkpointed')),
 execution_id TEXT REFERENCES code_execution(execution_id),
 execution_epoch INTEGER NOT NULL CHECK(execution_epoch>=1),
 version INTEGER NOT NULL CHECK(version>=1),
 CHECK(json_extract(instance_json,'$.computer_id')=computer_id AND json_extract(instance_json,'$.instance_epoch')=instance_epoch),
 CHECK((phase='checkpointed' AND execution_id IS NULL AND checkpoint_json IS NOT NULL) OR (phase!='checkpointed' AND execution_id IS NOT NULL))
) STRICT;
CREATE TRIGGER computer_version BEFORE UPDATE ON computer WHEN NEW.version!=OLD.version+1 OR NEW.computer_id!=OLD.computer_id OR NEW.instance_epoch<OLD.instance_epoch OR NEW.compatibility_json!=OLD.compatibility_json OR NEW.manifest_json!=OLD.manifest_json OR NOT(NEW.phase=OLD.phase OR (OLD.phase='restoring' AND NEW.phase IN ('ready','checkpointing')) OR (OLD.phase='ready' AND NEW.phase IN ('restoring','checkpointing')) OR (OLD.phase='checkpointing' AND NEW.phase IN ('checkpointed','restoring')) OR (OLD.phase='checkpointed' AND NEW.phase='restoring')) BEGIN SELECT RAISE(ABORT,'invalid Computer transition'); END;
CREATE TABLE computer_operation (
 operation_id TEXT PRIMARY KEY,
 execution_id TEXT NOT NULL REFERENCES code_execution(execution_id),
 execution_epoch INTEGER NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('ensure','idle')),
 request_json TEXT NOT NULL CHECK(json_valid(request_json)),
 previous_json TEXT CHECK(previous_json IS NULL OR json_valid(previous_json)),
 checkpoint_json TEXT CHECK(checkpoint_json IS NULL OR json_valid(checkpoint_json)),
 status TEXT NOT NULL CHECK(status IN ('accepted','completed','superseded')),
 UNIQUE(execution_id,execution_epoch,kind)
) STRICT;
CREATE TRIGGER computer_operation_frozen BEFORE UPDATE ON computer_operation WHEN NEW.operation_id!=OLD.operation_id OR NEW.execution_id!=OLD.execution_id OR NEW.execution_epoch!=OLD.execution_epoch OR NEW.kind!=OLD.kind OR NEW.request_json!=OLD.request_json OR NEW.previous_json IS NOT OLD.previous_json OR OLD.status!='accepted' BEGIN SELECT RAISE(ABORT,'frozen Computer operation'); END;
CREATE TRIGGER computer_operation_retained BEFORE DELETE ON computer_operation BEGIN SELECT RAISE(ABORT,'Computer operation retained'); END;
