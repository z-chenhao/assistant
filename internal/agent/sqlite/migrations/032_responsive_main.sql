-- Freeze asynchronous delivery semantics per Run. Old Runs retain their
-- original native ToolResult protocol; older binaries reject schema 32.
CREATE TABLE IF NOT EXISTS main_tool_protocol (
 run_id TEXT PRIMARY KEY REFERENCES run(run_id),
 version INTEGER NOT NULL DEFAULT 1 CHECK (version=1)
);

CREATE TRIGGER IF NOT EXISTS main_tool_protocol_initial BEFORE INSERT ON main_tool_protocol
 WHEN NOT EXISTS(SELECT 1 FROM run WHERE run_id=NEW.run_id AND kind='main' AND status='queued')
 BEGIN SELECT RAISE(ABORT,'invalid main tool protocol'); END;
CREATE TRIGGER IF NOT EXISTS main_tool_protocol_immutable BEFORE UPDATE ON main_tool_protocol
 BEGIN SELECT RAISE(ABORT,'immutable tool delivery protocol'); END;
CREATE TRIGGER IF NOT EXISTS main_tool_protocol_retained BEFORE DELETE ON main_tool_protocol
 BEGIN SELECT RAISE(ABORT,'immutable tool delivery protocol'); END;
