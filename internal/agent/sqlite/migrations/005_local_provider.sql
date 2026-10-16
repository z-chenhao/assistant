-- Retain configured endpoint locations independently of mutable profiles.
CREATE TABLE model_endpoint (endpoint TEXT PRIMARY KEY) STRICT;
INSERT INTO model_endpoint SELECT DISTINCT endpoint FROM model_profile WHERE endpoint IS NOT NULL;
CREATE TRIGGER model_profile_endpoint_insert AFTER INSERT ON model_profile WHEN NEW.endpoint IS NOT NULL
BEGIN INSERT OR IGNORE INTO model_endpoint(endpoint) VALUES(NEW.endpoint); END;
CREATE TRIGGER model_profile_endpoint_update AFTER UPDATE OF endpoint ON model_profile WHEN NEW.endpoint IS NOT NULL
BEGIN INSERT OR IGNORE INTO model_endpoint(endpoint) VALUES(NEW.endpoint); END;
CREATE TRIGGER model_endpoint_immutable_update BEFORE UPDATE ON model_endpoint BEGIN SELECT RAISE(ABORT, 'immutable endpoint'); END;
CREATE TRIGGER model_endpoint_immutable_delete BEFORE DELETE ON model_endpoint BEGIN SELECT RAISE(ABORT, 'immutable endpoint'); END;

-- Probe snapshots are also recorded here; they are not business Run snapshots.
CREATE TABLE provider_usage (
    generation_id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    snapshot_id TEXT NOT NULL,
    snapshot_json TEXT NOT NULL CHECK(json_valid(snapshot_json)),
    input_tokens INTEGER CHECK(input_tokens >= 0),
    output_tokens INTEGER CHECK(output_tokens >= 0),
    cache_read_tokens INTEGER CHECK(cache_read_tokens >= 0),
    cache_write_tokens INTEGER CHECK(cache_write_tokens >= 0),
    latency_ms INTEGER NOT NULL CHECK(latency_ms >= 0),
    error_code TEXT NOT NULL,
    recorded_at INTEGER NOT NULL
) STRICT;
CREATE TRIGGER provider_usage_immutable_update BEFORE UPDATE ON provider_usage BEGIN SELECT RAISE(ABORT, 'immutable usage'); END;
CREATE TRIGGER provider_usage_immutable_delete BEFORE DELETE ON provider_usage BEGIN SELECT RAISE(ABORT, 'immutable usage'); END;
