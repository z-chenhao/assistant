ALTER TABLE run_tree ADD COLUMN max_turns INTEGER NOT NULL DEFAULT 256 CHECK(max_turns BETWEEN 1 AND 512);
CREATE TRIGGER tree_turns_frozen BEFORE UPDATE ON run_tree WHEN NEW.max_turns!=OLD.max_turns
BEGIN SELECT RAISE(ABORT,'immutable tree turn ceiling'); END;
CREATE TABLE scheduler_tree (
 root_run_id TEXT PRIMARY KEY REFERENCES run_tree(root_run_id),
 last_served INTEGER NOT NULL DEFAULT 0 CHECK(last_served>=0)
) STRICT;
INSERT INTO scheduler_tree(root_run_id) SELECT root_run_id FROM run_tree;
CREATE TRIGGER scheduler_tree_create AFTER INSERT ON run_tree BEGIN
 INSERT INTO scheduler_tree(root_run_id,last_served) VALUES(NEW.root_run_id,(SELECT coalesce(max(last_served),0) FROM scheduler_tree));
END;
CREATE TRIGGER scheduler_tree_monotonic BEFORE UPDATE ON scheduler_tree WHEN NEW.root_run_id!=OLD.root_run_id OR NEW.last_served<=OLD.last_served
BEGIN SELECT RAISE(ABORT,'invalid scheduler turn'); END;
CREATE TRIGGER scheduler_tree_no_delete BEFORE DELETE ON scheduler_tree BEGIN SELECT RAISE(ABORT,'scheduler audit retained'); END;
CREATE TABLE scheduler_provider (
 provider_key TEXT PRIMARY KEY,
 consecutive_errors INTEGER NOT NULL DEFAULT 0 CHECK(consecutive_errors>=0),
 blocked_until INTEGER NOT NULL DEFAULT 0 CHECK(blocked_until>=0)
) STRICT;
CREATE INDEX run_root_position_status ON run(root_run_id,worker_position,status);
CREATE INDEX provider_usage_run ON provider_usage(run_id);
-- Read-only ledgers reuse immutable provider observations. Missing usage is
-- explicit; a provider that omits tokens never becomes a zero-cost claim.
CREATE VIEW run_tree_usage AS SELECT r.root_run_id,
 count(u.generation_id) AS model_calls,
 sum(u.input_tokens) AS input_tokens,sum(u.output_tokens) AS output_tokens,
 sum(u.cache_read_tokens) AS cache_read_tokens,sum(u.cache_write_tokens) AS cache_write_tokens,
 sum(CASE WHEN u.generation_id IS NOT NULL AND (u.input_tokens IS NULL OR u.output_tokens IS NULL) THEN 1 ELSE 0 END) AS missing_token_usage
 FROM run r LEFT JOIN provider_usage u USING(run_id) WHERE r.kind='worker' GROUP BY r.root_run_id;
CREATE VIEW task_usage AS SELECT r.task_id,count(u.generation_id) AS model_calls,
 sum(u.input_tokens) AS input_tokens,sum(u.output_tokens) AS output_tokens,
 sum(CASE WHEN u.generation_id IS NOT NULL AND (u.input_tokens IS NULL OR u.output_tokens IS NULL) THEN 1 ELSE 0 END) AS missing_token_usage
 FROM run r LEFT JOIN provider_usage u USING(run_id) WHERE r.kind='worker' GROUP BY r.task_id;
CREATE VIEW account_usage AS SELECT s.provider_kind||':'||coalesce(s.auth_connection_id,s.endpoint_hash,'local') AS provider_key,
 count(*) AS model_calls,sum(u.input_tokens) AS input_tokens,sum(u.output_tokens) AS output_tokens,
 sum(CASE WHEN u.input_tokens IS NULL OR u.output_tokens IS NULL THEN 1 ELSE 0 END) AS missing_token_usage
 FROM provider_usage u JOIN run r USING(run_id) JOIN model_snapshot s ON s.snapshot_id=r.model_snapshot_id
 GROUP BY provider_key;
