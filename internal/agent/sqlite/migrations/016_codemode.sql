-- M5 runtime transport and invocation-local Store. Source is immutable. Full
-- claim/wake and fixed-source recovery conformance is the following slice.
CREATE TABLE code_execution (
 execution_id TEXT PRIMARY KEY REFERENCES tool_invocation(invocation_id),
 source_hash TEXT NOT NULL, runtime_version TEXT NOT NULL, prelude_version TEXT NOT NULL,
 created_at INTEGER NOT NULL
) STRICT;
CREATE TRIGGER code_execution_immutable BEFORE UPDATE ON code_execution BEGIN SELECT RAISE(ABORT,'immutable code execution'); END;
CREATE TRIGGER code_execution_retained BEFORE DELETE ON code_execution BEGIN SELECT RAISE(ABORT,'code execution retained'); END;
CREATE TABLE code_store (
 execution_id TEXT NOT NULL REFERENCES code_execution(execution_id),
 key TEXT NOT NULL CHECK(length(key) BETWEEN 1 AND 256),
 value_json TEXT NOT NULL CHECK(json_valid(value_json) AND length(CAST(value_json AS BLOB))<=262144),
 version INTEGER NOT NULL CHECK(version>=1),
 PRIMARY KEY(execution_id,key)
) STRICT;
CREATE TRIGGER code_store_version BEFORE UPDATE ON code_store WHEN NEW.version!=OLD.version+1 OR NEW.execution_id!=OLD.execution_id OR NEW.key!=OLD.key BEGIN SELECT RAISE(ABORT,'invalid code store update'); END;
CREATE TABLE code_event (
 execution_id TEXT NOT NULL REFERENCES code_execution(execution_id),
 attempt_id TEXT NOT NULL, sequence INTEGER NOT NULL CHECK(sequence>0),
 message_json TEXT NOT NULL CHECK(json_valid(message_json)),
 PRIMARY KEY(execution_id,attempt_id,sequence)
) STRICT;
CREATE TRIGGER code_event_immutable BEFORE UPDATE ON code_event BEGIN SELECT RAISE(ABORT,'immutable code event'); END;
CREATE TRIGGER code_event_retained BEFORE DELETE ON code_event BEGIN SELECT RAISE(ABORT,'code event retained'); END;
CREATE TABLE code_call (
 execution_id TEXT NOT NULL REFERENCES code_execution(execution_id),
 call_id TEXT NOT NULL,
 invocation_id TEXT NOT NULL UNIQUE REFERENCES tool_invocation(invocation_id),
 request_hash TEXT NOT NULL,
 PRIMARY KEY(execution_id,call_id)
) STRICT;
CREATE TRIGGER code_call_immutable BEFORE UPDATE ON code_call BEGIN SELECT RAISE(ABORT,'immutable code call'); END;
CREATE TRIGGER code_call_retained BEFORE DELETE ON code_call BEGIN SELECT RAISE(ABORT,'code call retained'); END;
