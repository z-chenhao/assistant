CREATE TABLE memory_mutation (
 sequence INTEGER PRIMARY KEY, event_id TEXT NOT NULL UNIQUE,
 agent_id TEXT NOT NULL, operation TEXT NOT NULL CHECK(operation IN('observe','remember','forget')),
 source_type TEXT NOT NULL, source_ref TEXT NOT NULL, payload TEXT NOT NULL,
 accepted_at INTEGER NOT NULL, applied_at INTEGER, failed_at INTEGER, last_error TEXT, attempts INTEGER NOT NULL DEFAULT 0,
 UNIQUE(operation,source_type,source_ref)
) STRICT;
CREATE INDEX memory_pending ON memory_mutation(sequence) WHERE applied_at IS NULL AND failed_at IS NULL;
CREATE TRIGGER memory_mutation_identity BEFORE UPDATE ON memory_mutation WHEN
 NEW.event_id!=OLD.event_id OR NEW.agent_id!=OLD.agent_id OR NEW.operation!=OLD.operation OR
 NEW.source_type!=OLD.source_type OR NEW.source_ref!=OLD.source_ref OR NEW.payload!=OLD.payload OR NEW.accepted_at!=OLD.accepted_at
 BEGIN SELECT RAISE(ABORT,'immutable memory event'); END;
CREATE TABLE memory_privacy (
 agent_id TEXT PRIMARY KEY, epoch INTEGER NOT NULL DEFAULT 0,
 history_floor INTEGER NOT NULL DEFAULT 0, updated_at INTEGER NOT NULL
) STRICT;
CREATE TABLE run_memory_epoch(run_id TEXT PRIMARY KEY REFERENCES run(run_id),epoch INTEGER NOT NULL) STRICT;
CREATE TRIGGER run_memory_epoch AFTER INSERT ON run BEGIN
 INSERT INTO run_memory_epoch VALUES(NEW.run_id,coalesce((SELECT epoch FROM memory_privacy WHERE agent_id=NEW.agent_id),0));
END;
CREATE TABLE context_compaction (
 run_id TEXT NOT NULL REFERENCES run(run_id), step_index INTEGER NOT NULL,
 policy_hash TEXT NOT NULL, input_hash TEXT NOT NULL, messages_json TEXT NOT NULL CHECK(json_valid(messages_json)),
 provenance_json TEXT NOT NULL CHECK(json_valid(provenance_json)), created_at INTEGER NOT NULL,
 PRIMARY KEY(run_id,step_index)
) STRICT;
CREATE TABLE run_context_policy(run_id TEXT PRIMARY KEY REFERENCES run(run_id),policy_json TEXT NOT NULL CHECK(json_valid(policy_json))) STRICT;
INSERT INTO run_context_policy SELECT run_id,'{"threshold_percentage":0.75,"recent_messages":6,"summary_bytes":2048}' FROM run;
-- Accepted v18 requests retain their exact pre-compaction representation.
CREATE TABLE legacy_context_generation(generation_id TEXT PRIMARY KEY REFERENCES model_generation(generation_id)) STRICT;
INSERT INTO legacy_context_generation SELECT generation_id FROM model_generation;
CREATE TRIGGER context_policy_immutable BEFORE UPDATE ON run_context_policy BEGIN SELECT RAISE(ABORT,'immutable context policy'); END;
CREATE TABLE skill_candidate (
 candidate_id TEXT PRIMARY KEY,agent_id TEXT NOT NULL,name TEXT NOT NULL,description TEXT NOT NULL,procedure TEXT NOT NULL,
 reason TEXT NOT NULL CHECK(reason IN('correction','recovered_failure','success')),
 source_refs TEXT NOT NULL CHECK(json_valid(source_refs)),method_hash TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN('candidate','verified','promoted','rejected','retired')),
 created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL,version INTEGER NOT NULL DEFAULT 1,
 UNIQUE(agent_id,name,method_hash)
) STRICT;
CREATE TABLE skill_verification (
 candidate_id TEXT NOT NULL REFERENCES skill_candidate(candidate_id),invocation_id TEXT NOT NULL REFERENCES tool_invocation(invocation_id),
 run_id TEXT NOT NULL REFERENCES run(run_id),method_hash TEXT NOT NULL,created_at INTEGER NOT NULL,
 PRIMARY KEY(candidate_id,invocation_id),UNIQUE(candidate_id,run_id)
) STRICT;
CREATE TRIGGER skill_candidate_transition BEFORE UPDATE ON skill_candidate WHEN
 NEW.version!=OLD.version+1 OR NEW.candidate_id!=OLD.candidate_id OR NEW.agent_id!=OLD.agent_id OR NEW.name!=OLD.name
 OR NEW.description!=OLD.description OR NEW.procedure!=OLD.procedure OR NEW.reason!=OLD.reason OR NEW.source_refs!=OLD.source_refs
 OR NEW.method_hash!=OLD.method_hash OR NEW.created_at!=OLD.created_at
 OR NOT ((OLD.status='candidate' AND NEW.status IN('candidate','verified','rejected')) OR (OLD.status='verified' AND NEW.status IN('verified','promoted','rejected')) OR (OLD.status='promoted' AND NEW.status='retired'))
 BEGIN SELECT RAISE(ABORT,'invalid skill lifecycle'); END;
CREATE TRIGGER compaction_immutable BEFORE UPDATE ON context_compaction BEGIN SELECT RAISE(ABORT,'immutable compaction'); END;
CREATE TRIGGER compaction_audit BEFORE DELETE ON context_compaction BEGIN SELECT RAISE(ABORT,'retain compaction'); END;

DROP TRIGGER run_desired_state;
CREATE TRIGGER run_desired_state BEFORE UPDATE ON run WHEN NEW.desired_state NOT IN ('running','task_paused','task_cancelled','memory_forgotten')
 OR (OLD.desired_state!='running' AND NEW.desired_state='running')
 BEGIN SELECT RAISE(ABORT,'invalid desired state'); END;

CREATE TRIGGER run_privacy_immutable BEFORE UPDATE ON run_memory_epoch BEGIN SELECT RAISE(ABORT,'immutable privacy epoch'); END;
CREATE TRIGGER run_privacy_audit BEFORE DELETE ON run_memory_epoch BEGIN SELECT RAISE(ABORT,'retain privacy epoch'); END;
CREATE TRIGGER memory_privacy_transition BEFORE UPDATE ON memory_privacy WHEN NEW.agent_id!=OLD.agent_id OR NEW.epoch!=OLD.epoch+1 OR NEW.history_floor<OLD.history_floor OR NEW.updated_at<OLD.updated_at BEGIN SELECT RAISE(ABORT,'invalid privacy transition'); END;

CREATE TRIGGER compaction_no_replace BEFORE INSERT ON context_compaction WHEN EXISTS(SELECT 1 FROM context_compaction WHERE run_id=NEW.run_id AND step_index=NEW.step_index) BEGIN SELECT RAISE(ABORT,'compaction exists'); END;
CREATE TRIGGER skill_candidate_audit BEFORE DELETE ON skill_candidate BEGIN SELECT RAISE(ABORT,'retain skill candidate'); END;
CREATE TRIGGER skill_verification_immutable BEFORE UPDATE ON skill_verification BEGIN SELECT RAISE(ABORT,'immutable skill verification'); END;
CREATE TRIGGER skill_verification_audit BEFORE DELETE ON skill_verification BEGIN SELECT RAISE(ABORT,'retain skill verification'); END;
