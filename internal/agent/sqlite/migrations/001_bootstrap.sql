-- Only the no-profile state exists in this slice. A later model setup migration
-- must introduce verified profiles before it can relax these constraints.
CREATE TABLE agent_config (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    agent_id TEXT NOT NULL UNIQUE,
    active_model_profile_id TEXT CHECK (active_model_profile_id IS NULL),
    setup_status TEXT NOT NULL CHECK (setup_status = 'setup_required'),
    config_version INTEGER NOT NULL CHECK (config_version >= 1),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL CHECK (updated_at >= created_at)
) STRICT;
