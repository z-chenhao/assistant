-- Host-only ephemeral bindings are not part of a Computer checkpoint. The
-- retained ledger is scoped to one Code execution and fenced by its live claim.
CREATE TABLE computer_use (
 execution_id TEXT PRIMARY KEY REFERENCES code_execution(execution_id),
 state_json TEXT NOT NULL CHECK(json_valid(state_json)),
 version INTEGER NOT NULL CHECK(version>=1)
) STRICT;
CREATE TRIGGER computer_use_version BEFORE UPDATE ON computer_use
 WHEN NEW.execution_id!=OLD.execution_id OR NEW.version!=OLD.version+1
 BEGIN SELECT RAISE(ABORT,'invalid Computer Use version'); END;
CREATE TABLE computer_lease_resume (
 execution_id TEXT PRIMARY KEY REFERENCES code_execution(execution_id),
 execution_epoch INTEGER NOT NULL CHECK(execution_epoch>=1)
) STRICT;
CREATE TABLE computer_file (
 computer_id TEXT NOT NULL REFERENCES computer(computer_id),
 path TEXT NOT NULL,
 agent_id TEXT NOT NULL,
 task_id TEXT,
 run_id TEXT NOT NULL REFERENCES run(run_id),
 PRIMARY KEY(computer_id,path)
) STRICT;
