-- Extend optional dispatch diagnostics while retaining every recorded HTTP attempt.
ALTER TABLE dispatch_observation RENAME TO previous_dispatch_observation;

CREATE TABLE dispatch_observation (
 generation_id TEXT NOT NULL REFERENCES model_generation(generation_id),
 attempt TEXT NOT NULL, phase TEXT NOT NULL CHECK(phase IN (
  'started','headers','error','first_body','first_text','first_tool_call','stream_terminal',
  'header_timeout','stream_idle_timeout','generation_deadline'
 )),
 occurred_at INTEGER NOT NULL, format TEXT NOT NULL, body_json TEXT CHECK(body_json IS NULL OR json_valid(body_json)),
 body_hash TEXT NOT NULL, omitted TEXT NOT NULL, http_status INTEGER NOT NULL, error_code TEXT NOT NULL,
 PRIMARY KEY(generation_id,attempt,phase)
) STRICT;

INSERT INTO dispatch_observation SELECT * FROM previous_dispatch_observation;
DROP TABLE previous_dispatch_observation;
CREATE INDEX dispatch_observation_time ON dispatch_observation(generation_id,occurred_at,attempt);
