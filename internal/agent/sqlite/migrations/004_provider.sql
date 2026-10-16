-- Preserve identity and timestamps while replacing M0's closed-only config.
ALTER TABLE agent_config RENAME TO agent_config_v3;
CREATE TABLE agent_config (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    agent_id TEXT NOT NULL UNIQUE,
    active_model_profile_id TEXT REFERENCES model_profile(profile_id),
    setup_status TEXT NOT NULL CHECK (setup_status IN ('setup_required','ready','degraded')),
    config_version INTEGER NOT NULL CHECK (config_version >= 1),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL CHECK (updated_at >= created_at),
    CHECK ((active_model_profile_id IS NULL AND setup_status = 'setup_required') OR
           (active_model_profile_id IS NOT NULL AND setup_status IN ('ready','degraded')))
) STRICT;
INSERT INTO agent_config SELECT * FROM agent_config_v3;
DROP TABLE agent_config_v3;

CREATE TABLE model_probe (
    profile_id TEXT PRIMARY KEY REFERENCES model_profile(profile_id),
    profile_version INTEGER NOT NULL CHECK (profile_version >= 1),
    auth_version INTEGER NOT NULL CHECK (auth_version >= 0),
    bridge_identity TEXT NOT NULL CHECK (length(bridge_identity) > 0),
    result_json TEXT NOT NULL CHECK (json_valid(result_json)),
    verified_at INTEGER NOT NULL
) STRICT;

CREATE TRIGGER agent_config_select_ready BEFORE UPDATE OF active_model_profile_id,setup_status ON agent_config
WHEN NEW.setup_status = 'ready' AND NOT EXISTS (
    SELECT 1 FROM model_profile p JOIN model_probe v ON v.profile_id=p.profile_id
    WHERE p.profile_id=NEW.active_model_profile_id AND p.status='ready'
      AND p.verified_at IS NOT NULL AND v.profile_version=p.version
      AND (p.auth_connection_id IS NULL OR EXISTS (
          SELECT 1 FROM auth_connection a WHERE a.connection_id=p.auth_connection_id
            AND a.status='connected' AND a.version=v.auth_version)))
BEGIN SELECT RAISE(ABORT, 'active profile requires current verification'); END;

CREATE TRIGGER agent_config_version_update BEFORE UPDATE ON agent_config
WHEN NEW.config_version != OLD.config_version+1 OR NEW.agent_id != OLD.agent_id
  OR NEW.created_at != OLD.created_at OR NEW.updated_at < OLD.updated_at
BEGIN SELECT RAISE(ABORT, 'invalid agent configuration version'); END;
