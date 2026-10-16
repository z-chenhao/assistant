-- Additive migration: identity, closed ingress, and existing Outbox/Inbox rows
-- are untouched. Activation remains unavailable until the Probe slice.
CREATE TABLE auth_connection (
    connection_id TEXT PRIMARY KEY,
    provider TEXT NOT NULL CHECK (length(trim(provider)) > 0),
    auth_kind TEXT NOT NULL CHECK (auth_kind IN ('api_key', 'chatgpt_oauth')),
    credential_ref TEXT NOT NULL UNIQUE CHECK (length(credential_ref) > 0),
    account_label TEXT,
    workspace_label TEXT,
    status TEXT NOT NULL CHECK (status IN ('connected', 'refreshing', 'reauth_required', 'logged_out')),
    expires_at INTEGER,
    last_refreshed_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL CHECK (updated_at >= created_at),
    version INTEGER NOT NULL CHECK (version >= 1),
    CHECK (auth_kind != 'chatgpt_oauth' OR provider = 'openai')
) STRICT;
CREATE INDEX auth_connection_provider_status ON auth_connection(provider, status);

CREATE TABLE model_profile (
    profile_id TEXT PRIMARY KEY,
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    provider_kind TEXT NOT NULL CHECK (provider_kind IN ('openai_compatible', 'openai_account')),
    endpoint TEXT,
    wire_api TEXT NOT NULL CHECK (wire_api IN ('openai_completions', 'openai_responses')),
    model_id TEXT NOT NULL CHECK (length(trim(model_id)) > 0),
    auth_connection_id TEXT REFERENCES auth_connection(connection_id),
    reasoning_effort TEXT,
    context_window INTEGER NOT NULL CHECK (context_window > 0),
    max_output_tokens INTEGER NOT NULL CHECK (max_output_tokens > 0 AND max_output_tokens <= context_window),
    generation_defaults_json TEXT NOT NULL CHECK (json_valid(generation_defaults_json) AND json_type(generation_defaults_json) = 'object'),
    compatibility_json TEXT NOT NULL CHECK (json_valid(compatibility_json) AND json_type(compatibility_json) = 'object'),
    capabilities_json TEXT NOT NULL CHECK (json_valid(capabilities_json) AND json_type(capabilities_json) = 'object'),
    status TEXT NOT NULL CHECK (status IN ('unverified', 'ready', 'unavailable', 'reauth_required', 'invalid')),
    verified_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL CHECK (updated_at >= created_at),
    version INTEGER NOT NULL CHECK (version >= 1),
    CHECK (status != 'ready' OR verified_at IS NOT NULL),
    CHECK ((provider_kind = 'openai_compatible' AND endpoint IS NOT NULL AND length(endpoint) > 0)
        OR (provider_kind = 'openai_account' AND endpoint IS NULL AND auth_connection_id IS NOT NULL AND wire_api = 'openai_responses'))
) STRICT;
CREATE INDEX model_profile_provider_model_endpoint ON model_profile(provider_kind, model_id, endpoint);

CREATE TRIGGER model_profile_auth_insert BEFORE INSERT ON model_profile
WHEN NEW.auth_connection_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM auth_connection a WHERE a.connection_id = NEW.auth_connection_id AND
    ((NEW.provider_kind = 'openai_compatible' AND a.auth_kind = 'api_key') OR
     (NEW.provider_kind = 'openai_account' AND a.auth_kind = 'chatgpt_oauth' AND a.provider = 'openai')))
BEGIN SELECT RAISE(ABORT, 'invalid profile authentication relationship'); END;
CREATE TRIGGER model_profile_auth_update BEFORE UPDATE ON model_profile
WHEN NEW.auth_connection_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM auth_connection a WHERE a.connection_id = NEW.auth_connection_id AND
    ((NEW.provider_kind = 'openai_compatible' AND a.auth_kind = 'api_key') OR
     (NEW.provider_kind = 'openai_account' AND a.auth_kind = 'chatgpt_oauth' AND a.provider = 'openai')))
BEGIN SELECT RAISE(ABORT, 'invalid profile authentication relationship'); END;

CREATE TRIGGER auth_connection_version_insert BEFORE INSERT ON auth_connection WHEN NEW.version != 1
BEGIN SELECT RAISE(ABORT, 'initial auth version must be 1'); END;
CREATE TRIGGER auth_connection_version_update BEFORE UPDATE ON auth_connection
WHEN NEW.version != OLD.version + 1 OR NEW.connection_id != OLD.connection_id OR NEW.created_at != OLD.created_at
  OR NEW.provider != OLD.provider OR NEW.auth_kind != OLD.auth_kind OR NEW.credential_ref != OLD.credential_ref
  OR NEW.updated_at < OLD.updated_at
BEGIN SELECT RAISE(ABORT, 'invalid auth version or identity'); END;
CREATE TRIGGER model_profile_version_insert BEFORE INSERT ON model_profile WHEN NEW.version != 1
BEGIN SELECT RAISE(ABORT, 'initial profile version must be 1'); END;
CREATE TRIGGER model_profile_version_update BEFORE UPDATE ON model_profile
WHEN NEW.version != OLD.version + 1 OR NEW.profile_id != OLD.profile_id OR NEW.created_at != OLD.created_at OR NEW.updated_at < OLD.updated_at
BEGIN SELECT RAISE(ABORT, 'invalid profile version or identity'); END;

CREATE TABLE model_snapshot (
    snapshot_id TEXT PRIMARY KEY,
    profile_id TEXT NOT NULL REFERENCES model_profile(profile_id),
    profile_version INTEGER NOT NULL CHECK (profile_version >= 1),
    provider_kind TEXT NOT NULL CHECK (provider_kind IN ('openai_compatible', 'openai_account')),
    endpoint_hash TEXT CHECK (endpoint_hash IS NULL OR (length(endpoint_hash) = 64 AND endpoint_hash NOT GLOB '*[^0-9a-f]*')),
    wire_api TEXT NOT NULL CHECK (wire_api IN ('openai_completions', 'openai_responses')),
    model_id TEXT NOT NULL,
    auth_connection_id TEXT REFERENCES auth_connection(connection_id),
    reasoning_effort TEXT,
    context_window INTEGER NOT NULL CHECK (context_window > 0),
    max_output_tokens INTEGER NOT NULL CHECK (max_output_tokens > 0 AND max_output_tokens <= context_window),
    generation_defaults_json TEXT NOT NULL CHECK (json_valid(generation_defaults_json) AND json_type(generation_defaults_json) = 'object'),
    compatibility_json TEXT NOT NULL CHECK (json_valid(compatibility_json) AND json_type(compatibility_json) = 'object'),
    capabilities_json TEXT NOT NULL CHECK (json_valid(capabilities_json) AND json_type(capabilities_json) = 'object'),
    created_at INTEGER NOT NULL
) STRICT;
CREATE TRIGGER model_snapshot_no_update BEFORE UPDATE ON model_snapshot
BEGIN SELECT RAISE(ABORT, 'model snapshots are immutable'); END;
CREATE TRIGGER model_snapshot_no_delete BEFORE DELETE ON model_snapshot
BEGIN SELECT RAISE(ABORT, 'model snapshots are immutable'); END;
-- REPLACE does not fire delete triggers by default. Reject it before insertion.
CREATE TRIGGER model_snapshot_no_replace BEFORE INSERT ON model_snapshot
WHEN EXISTS (SELECT 1 FROM model_snapshot WHERE snapshot_id = NEW.snapshot_id)
BEGIN SELECT RAISE(ABORT, 'model snapshots are immutable'); END;
CREATE TRIGGER model_profile_no_replace BEFORE INSERT ON model_profile
WHEN EXISTS (SELECT 1 FROM model_profile WHERE profile_id = NEW.profile_id)
BEGIN SELECT RAISE(ABORT, 'profile identity already exists'); END;
CREATE TRIGGER auth_connection_no_replace BEFORE INSERT ON auth_connection
WHEN EXISTS (SELECT 1 FROM auth_connection WHERE connection_id = NEW.connection_id)
BEGIN SELECT RAISE(ABORT, 'auth identity already exists'); END;
