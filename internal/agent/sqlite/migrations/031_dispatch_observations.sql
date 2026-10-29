-- Optional best-effort diagnostics. These rows never drive execution/replay.
CREATE TABLE dispatch_observation (
 generation_id TEXT NOT NULL REFERENCES model_generation(generation_id),
 attempt TEXT NOT NULL, phase TEXT NOT NULL CHECK(phase IN ('started','headers','error')),
 occurred_at INTEGER NOT NULL, format TEXT NOT NULL, body_json TEXT CHECK(body_json IS NULL OR json_valid(body_json)),
 body_hash TEXT NOT NULL, omitted TEXT NOT NULL, http_status INTEGER NOT NULL, error_code TEXT NOT NULL,
 PRIMARY KEY(generation_id,attempt,phase)
) STRICT;
CREATE INDEX dispatch_observation_time ON dispatch_observation(generation_id,occurred_at,attempt);

CREATE INDEX run_observation_root ON run(root_run_id,created_at,run_id);
