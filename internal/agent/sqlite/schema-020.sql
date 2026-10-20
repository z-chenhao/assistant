-- Generated empty schema v20; migrations SHA-256: fbe9bc52e7727236d16aac258b6914d84912020b958adf42853d294cfe820b24
-- Update: go test ./internal/agent/sqlite -run '^TestFreshSchemaSnapshotMatchesHistory$' -args -update-state-schema
CREATE TABLE event_outbox (
    outbox_id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL UNIQUE,
    subject TEXT NOT NULL CHECK (length(subject) > 0),
    type TEXT NOT NULL CHECK (length(type) > 0),
    payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
    occurred_at INTEGER NOT NULL,
    published_at INTEGER,
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0)
) STRICT;
CREATE INDEX event_outbox_pending ON event_outbox(outbox_id) WHERE published_at IS NULL;
CREATE TABLE event_inbox (
    consumer TEXT NOT NULL CHECK (length(consumer) > 0),
    event_id TEXT NOT NULL,
    processed_at INTEGER NOT NULL,
    PRIMARY KEY (consumer, event_id)
) STRICT;
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
CREATE TRIGGER model_snapshot_no_replace BEFORE INSERT ON model_snapshot
WHEN EXISTS (SELECT 1 FROM model_snapshot WHERE snapshot_id = NEW.snapshot_id)
BEGIN SELECT RAISE(ABORT, 'model snapshots are immutable'); END;
CREATE TRIGGER model_profile_no_replace BEFORE INSERT ON model_profile
WHEN EXISTS (SELECT 1 FROM model_profile WHERE profile_id = NEW.profile_id)
BEGIN SELECT RAISE(ABORT, 'profile identity already exists'); END;
CREATE TRIGGER auth_connection_no_replace BEFORE INSERT ON auth_connection
WHEN EXISTS (SELECT 1 FROM auth_connection WHERE connection_id = NEW.connection_id)
BEGIN SELECT RAISE(ABORT, 'auth identity already exists'); END;
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
CREATE TABLE model_endpoint (endpoint TEXT PRIMARY KEY) STRICT;
CREATE TRIGGER model_profile_endpoint_insert AFTER INSERT ON model_profile WHEN NEW.endpoint IS NOT NULL
BEGIN INSERT OR IGNORE INTO model_endpoint(endpoint) VALUES(NEW.endpoint); END;
CREATE TRIGGER model_profile_endpoint_update AFTER UPDATE OF endpoint ON model_profile WHEN NEW.endpoint IS NOT NULL
BEGIN INSERT OR IGNORE INTO model_endpoint(endpoint) VALUES(NEW.endpoint); END;
CREATE TRIGGER model_endpoint_immutable_update BEFORE UPDATE ON model_endpoint BEGIN SELECT RAISE(ABORT, 'immutable endpoint'); END;
CREATE TRIGGER model_endpoint_immutable_delete BEFORE DELETE ON model_endpoint BEGIN SELECT RAISE(ABORT, 'immutable endpoint'); END;
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
CREATE TABLE installation_auth (
    singleton INTEGER PRIMARY KEY CHECK(singleton=1),
    device_id TEXT NOT NULL CHECK(length(device_id)=36)
) STRICT;
CREATE TABLE account_registration (
    connection_id TEXT PRIMARY KEY REFERENCES auth_connection(connection_id),
    client_id TEXT NOT NULL,
    subject_hash TEXT NOT NULL CHECK(length(subject_hash)=64),
    auth_epoch INTEGER NOT NULL DEFAULT 1 CHECK(auth_epoch>=1),
    UNIQUE(client_id,subject_hash)
) STRICT;
CREATE TRIGGER installation_auth_stable_update BEFORE UPDATE ON installation_auth
BEGIN SELECT RAISE(ABORT,'stable installation identity'); END;
CREATE TRIGGER installation_auth_stable_delete BEFORE DELETE ON installation_auth
BEGIN SELECT RAISE(ABORT,'stable installation identity'); END;
CREATE TRIGGER account_registration_immutable BEFORE UPDATE ON account_registration
WHEN NEW.connection_id!=OLD.connection_id OR NEW.client_id!=OLD.client_id OR NEW.subject_hash!=OLD.subject_hash OR NEW.auth_epoch!=OLD.auth_epoch+1
BEGIN SELECT RAISE(ABORT,'immutable account registration'); END;
CREATE TRIGGER account_registration_no_replace BEFORE INSERT ON account_registration
WHEN EXISTS(SELECT 1 FROM account_registration WHERE connection_id=NEW.connection_id)
BEGIN SELECT RAISE(ABORT,'immutable account registration'); END;
CREATE TRIGGER account_registration_no_delete BEFORE DELETE ON account_registration
BEGIN SELECT RAISE(ABORT,'immutable account registration'); END;
CREATE TRIGGER auth_connection_legal_transition BEFORE UPDATE ON auth_connection
WHEN NOT (
 (OLD.status='connected' AND NEW.status IN ('connected','refreshing','reauth_required','logged_out')) OR
 (OLD.status='refreshing' AND NEW.status IN ('connected','reauth_required','logged_out')) OR
 (OLD.status='reauth_required' AND NEW.status IN ('connected','logged_out')) OR
 (OLD.status='logged_out' AND NEW.status='connected'))
BEGIN SELECT RAISE(ABORT,'illegal authentication transition'); END;
CREATE TABLE capability_snapshot (
 snapshot_id TEXT PRIMARY KEY, agent_config_version INTEGER NOT NULL, role TEXT NOT NULL CHECK(role IN ('main','worker')),
 tool_specs_hash TEXT NOT NULL, skill_set_hash TEXT NOT NULL, data_scope TEXT NOT NULL,
 can_spawn_agents INTEGER NOT NULL CHECK(can_spawn_agents IN (0,1)), tools_json TEXT NOT NULL CHECK(json_valid(tools_json)), created_at INTEGER NOT NULL
, format_version INTEGER NOT NULL DEFAULT 1 CHECK(format_version IN (1,2))) STRICT;
CREATE TRIGGER capability_no_update BEFORE UPDATE ON capability_snapshot BEGIN SELECT RAISE(ABORT,'immutable capability'); END;
CREATE TRIGGER capability_no_delete BEFORE DELETE ON capability_snapshot BEGIN SELECT RAISE(ABORT,'immutable capability'); END;
CREATE TRIGGER capability_no_replace BEFORE INSERT ON capability_snapshot WHEN EXISTS(SELECT 1 FROM capability_snapshot WHERE snapshot_id=NEW.snapshot_id) BEGIN SELECT RAISE(ABORT,'immutable capability'); END;
CREATE TABLE run (
 run_id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, conversation_id TEXT NOT NULL,
 task_id TEXT, parent_run_id TEXT REFERENCES run(run_id), root_run_id TEXT REFERENCES run(run_id),
 kind TEXT NOT NULL CHECK(kind IN ('main','worker')), worker_position TEXT CHECK(worker_position IN ('root','child')),
 status TEXT NOT NULL CHECK(status IN ('queued','running','waiting','completed','failed','cancelled')),
 wait_reason TEXT CHECK(wait_reason IN ('user_input','approval','external_event','resource')),
 task_epoch INTEGER, depth INTEGER NOT NULL DEFAULT 0,
 capability_snapshot_id TEXT NOT NULL REFERENCES capability_snapshot(snapshot_id), model_snapshot_id TEXT NOT NULL REFERENCES model_snapshot(snapshot_id),
 desired_state TEXT NOT NULL DEFAULT 'running', checkpoint_ref TEXT, result_ref TEXT, error_code TEXT,
 checkpoint_json TEXT NOT NULL CHECK(json_valid(checkpoint_json)), history_boundary INTEGER NOT NULL,
 started_at INTEGER, ended_at INTEGER, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, version INTEGER NOT NULL CHECK(version>=1),
 CHECK((kind='main' AND task_id IS NULL AND parent_run_id IS NULL AND worker_position IS NULL AND depth=0) OR
       (kind='worker' AND task_id IS NOT NULL AND ((worker_position='root' AND parent_run_id IS NULL) OR (worker_position='child' AND parent_run_id IS NOT NULL)))),
 CHECK((status='waiting')=(wait_reason IS NOT NULL)),
 CHECK((status IN ('completed','failed','cancelled'))=(ended_at IS NOT NULL))
) STRICT;
CREATE UNIQUE INDEX one_root ON run(task_id) WHERE kind='worker' AND worker_position='root' AND status IN ('queued','running','waiting');
CREATE INDEX run_parent_status ON run(parent_run_id,status);
CREATE INDEX run_task_status ON run(task_id,status);
CREATE INDEX run_recovery ON run(status,wait_reason);
CREATE TRIGGER run_no_delete BEFORE DELETE ON run BEGIN SELECT RAISE(ABORT,'run audit retained'); END;
CREATE TRIGGER run_no_replace BEFORE INSERT ON run WHEN EXISTS(SELECT 1 FROM run WHERE run_id=NEW.run_id) BEGIN SELECT RAISE(ABORT,'run identity exists'); END;
CREATE TRIGGER run_initial BEFORE INSERT ON run WHEN NEW.version!=1 OR NEW.status!='queued' BEGIN SELECT RAISE(ABORT,'run starts queued at version one'); END;
CREATE TABLE run_claim (
 run_id TEXT PRIMARY KEY REFERENCES run(run_id), epoch INTEGER NOT NULL, token TEXT, expires_at INTEGER NOT NULL, version INTEGER NOT NULL
) STRICT;
CREATE INDEX run_claim_expiry ON run_claim(expires_at);
CREATE TABLE main_input (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT, agent_id TEXT NOT NULL, request_key TEXT NOT NULL,
 run_id TEXT NOT NULL REFERENCES run(run_id), message_id TEXT NOT NULL UNIQUE, message_json TEXT NOT NULL CHECK(json_valid(message_json)),
 UNIQUE(agent_id,request_key)
) STRICT;
CREATE INDEX main_input_run_sequence ON main_input(run_id,sequence);
CREATE TRIGGER input_no_update BEFORE UPDATE ON main_input BEGIN SELECT RAISE(ABORT,'immutable input'); END;
CREATE TRIGGER input_no_delete BEFORE DELETE ON main_input BEGIN SELECT RAISE(ABORT,'input audit retained'); END;
CREATE TABLE conversation_intent (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT, message_id TEXT NOT NULL UNIQUE, message_json TEXT NOT NULL CHECK(json_valid(message_json)), flushed INTEGER NOT NULL DEFAULT 0
) STRICT;
CREATE TRIGGER conversation_intent_identity BEFORE UPDATE ON conversation_intent WHEN NEW.sequence!=OLD.sequence OR NEW.message_id!=OLD.message_id OR NEW.message_json!=OLD.message_json OR NEW.flushed<OLD.flushed BEGIN SELECT RAISE(ABORT,'immutable message intent'); END;
CREATE TRIGGER intent_no_delete BEFORE DELETE ON conversation_intent BEGIN SELECT RAISE(ABORT,'message audit retained'); END;
CREATE TABLE model_generation (
 generation_id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES run(run_id), step_index INTEGER NOT NULL,
 request_json TEXT NOT NULL CHECK(json_valid(request_json)), request_hash TEXT NOT NULL, owner_token TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','completed','failed')), result_json TEXT, error_code TEXT NOT NULL DEFAULT '',
 attempt_count INTEGER NOT NULL DEFAULT 1, created_at INTEGER NOT NULL, completed_at INTEGER, UNIQUE(run_id,step_index)
) STRICT;
CREATE TRIGGER generation_frozen BEFORE UPDATE ON model_generation WHEN OLD.status!='pending' OR NEW.request_json!=OLD.request_json OR NEW.request_hash!=OLD.request_hash OR NEW.generation_id!=OLD.generation_id OR NEW.run_id!=OLD.run_id OR NEW.step_index!=OLD.step_index BEGIN SELECT RAISE(ABORT,'immutable model exchange'); END;
CREATE TRIGGER generation_no_delete BEFORE DELETE ON model_generation BEGIN SELECT RAISE(ABORT,'generation audit retained'); END;
CREATE TRIGGER generation_no_replace BEFORE INSERT ON model_generation WHEN EXISTS(SELECT 1 FROM model_generation WHERE generation_id=NEW.generation_id) BEGIN SELECT RAISE(ABORT,'immutable model exchange'); END;
CREATE TABLE tool_invocation (
 invocation_id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES run(run_id), parent_invocation_id TEXT REFERENCES tool_invocation(invocation_id),
 provider_exchange_id TEXT NOT NULL REFERENCES model_generation(generation_id), provider_call_id TEXT NOT NULL,
 tool_name TEXT NOT NULL, arguments_json TEXT NOT NULL, request_hash TEXT NOT NULL, replay_policy TEXT NOT NULL CHECK(replay_policy IN ('safe','query_before_retry','never')),
 idempotency_key TEXT NOT NULL UNIQUE, status TEXT NOT NULL CHECK(status IN ('queued','running','waiting','succeeded','failed','cancelled','unknown')),
 wait_reason TEXT, attempt_count INTEGER NOT NULL DEFAULT 0, result_ref TEXT, error_json TEXT, result_json TEXT,
 result_ready_at INTEGER, delivery_seq INTEGER NOT NULL, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, version INTEGER NOT NULL, desired_state TEXT NOT NULL DEFAULT 'running' CHECK(desired_state IN ('running','cancelled')), policy_json TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(policy_json)), conflict_keys TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(conflict_keys)), next_wake_at INTEGER, execution_token TEXT, reconciliation_json TEXT CHECK(reconciliation_json IS NULL OR json_valid(reconciliation_json)),
 UNIQUE(run_id,provider_exchange_id,provider_call_id)
) STRICT;
CREATE TRIGGER invocation_no_delete BEFORE DELETE ON tool_invocation BEGIN SELECT RAISE(ABORT,'tool audit retained'); END;
CREATE TRIGGER invocation_no_replace BEFORE INSERT ON tool_invocation WHEN EXISTS(SELECT 1 FROM tool_invocation WHERE invocation_id=NEW.invocation_id) BEGIN SELECT RAISE(ABORT,'tool identity exists'); END;
CREATE TABLE main_reply (
 exchange_id TEXT PRIMARY KEY REFERENCES model_generation(generation_id), run_id TEXT NOT NULL REFERENCES run(run_id), message_id TEXT NOT NULL UNIQUE, delivery_id TEXT NOT NULL UNIQUE
) STRICT;
CREATE TABLE run_trace (
 run_id TEXT NOT NULL REFERENCES run(run_id), sequence INTEGER NOT NULL, event_json TEXT NOT NULL CHECK(json_valid(event_json)), occurred_at INTEGER NOT NULL,
 PRIMARY KEY(run_id,sequence)
) STRICT;
CREATE TABLE task (
 task_id TEXT PRIMARY KEY, task_ref TEXT NOT NULL UNIQUE, agent_id TEXT NOT NULL, conversation_id TEXT NOT NULL,
 title TEXT NOT NULL, anchor_task TEXT NOT NULL, anchor_artifact_refs TEXT NOT NULL CHECK(json_valid(anchor_artifact_refs)),
 status TEXT NOT NULL CHECK(status IN ('active','paused','completed','cancelled')), task_epoch INTEGER NOT NULL CHECK(task_epoch>=1),
 progress_summary TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
 completed_at INTEGER, cancelled_at INTEGER, version INTEGER NOT NULL CHECK(version>=1), completion_policy TEXT NOT NULL DEFAULT 'once' CHECK(completion_policy IN ('once','ongoing')),
 CHECK((status='completed')=(completed_at IS NOT NULL)), CHECK((status='cancelled')=(cancelled_at IS NOT NULL))
) STRICT;
CREATE INDEX task_agent_status ON task(agent_id,status,updated_at);
CREATE TRIGGER task_no_delete BEFORE DELETE ON task BEGIN SELECT RAISE(ABORT,'task audit retained'); END;
CREATE TRIGGER task_no_replace BEFORE INSERT ON task WHEN EXISTS(SELECT 1 FROM task WHERE task_id=NEW.task_id) BEGIN SELECT RAISE(ABORT,'task identity exists'); END;
CREATE TRIGGER task_initial BEFORE INSERT ON task WHEN NEW.version!=1 OR NEW.status!='active' OR NEW.task_epoch!=1 BEGIN SELECT RAISE(ABORT,'task starts active at epoch/version one'); END;
CREATE TRIGGER worker_task_initial BEFORE INSERT ON run WHEN NEW.kind='worker' AND NOT EXISTS(SELECT 1 FROM task t WHERE t.task_id=NEW.task_id AND t.agent_id=NEW.agent_id AND t.conversation_id=NEW.conversation_id AND t.task_epoch=NEW.task_epoch AND t.status='active') BEGIN SELECT RAISE(ABORT,'invalid worker task scope'); END;
CREATE TABLE task_instruction (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT, instruction_id TEXT NOT NULL UNIQUE, task_id TEXT NOT NULL REFERENCES task(task_id),
 task_epoch INTEGER NOT NULL, source_message_id TEXT, instruction TEXT NOT NULL, artifact_refs TEXT NOT NULL CHECK(json_valid(artifact_refs)), created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX task_instruction_epoch ON task_instruction(task_id,task_epoch,sequence);
CREATE TRIGGER instruction_no_update BEFORE UPDATE ON task_instruction BEGIN SELECT RAISE(ABORT,'immutable task instruction'); END;
CREATE TRIGGER instruction_no_delete BEFORE DELETE ON task_instruction BEGIN SELECT RAISE(ABORT,'instruction audit retained'); END;
CREATE TABLE task_request (
 source_message_id TEXT NOT NULL, request_hash TEXT NOT NULL, receipt_json TEXT NOT NULL CHECK(json_valid(receipt_json)),
 invocation_id TEXT NOT NULL UNIQUE REFERENCES tool_invocation(invocation_id), PRIMARY KEY(source_message_id,request_hash)
) STRICT;
CREATE TABLE task_invocation (
 invocation_id TEXT PRIMARY KEY REFERENCES tool_invocation(invocation_id), request_hash TEXT NOT NULL, receipt_json TEXT NOT NULL CHECK(json_valid(receipt_json))
) STRICT;
CREATE TRIGGER task_request_no_update BEFORE UPDATE ON task_request BEGIN SELECT RAISE(ABORT,'immutable task request'); END;
CREATE TRIGGER task_invocation_no_update BEFORE UPDATE ON task_invocation BEGIN SELECT RAISE(ABORT,'immutable task invocation'); END;
CREATE TABLE worker_result (
 run_id TEXT NOT NULL REFERENCES run(run_id), exchange_id TEXT PRIMARY KEY REFERENCES model_generation(generation_id),
 content TEXT NOT NULL, created_at INTEGER NOT NULL
) STRICT;
CREATE TRIGGER result_no_update BEFORE UPDATE ON worker_result BEGIN SELECT RAISE(ABORT,'immutable worker result'); END;
CREATE TABLE task_update (
 event_id TEXT PRIMARY KEY, task_id TEXT NOT NULL REFERENCES task(task_id), root_run_id TEXT NOT NULL REFERENCES run(run_id),
 task_epoch INTEGER NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('completed','failed','needs_input','unknown')),
 content TEXT NOT NULL, artifact_refs TEXT NOT NULL CHECK(json_valid(artifact_refs)), occurred_at INTEGER NOT NULL,
 UNIQUE(root_run_id,kind)
) STRICT;
CREATE TRIGGER task_update_no_update BEFORE UPDATE ON task_update BEGIN SELECT RAISE(ABORT,'immutable task update'); END;
CREATE TRIGGER task_update_no_delete BEFORE DELETE ON task_update BEGIN SELECT RAISE(ABORT,'task update audit retained'); END;
CREATE TABLE main_inbox (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT, event_id TEXT NOT NULL UNIQUE REFERENCES task_update(event_id),
 request_id TEXT NOT NULL UNIQUE, agent_id TEXT NOT NULL, run_id TEXT REFERENCES run(run_id), message_json TEXT NOT NULL CHECK(json_valid(message_json)), created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX inbox_pending ON main_inbox(agent_id,sequence) WHERE run_id IS NULL;
CREATE TRIGGER inbox_identity BEFORE UPDATE ON main_inbox WHEN NEW.event_id!=OLD.event_id OR NEW.request_id!=OLD.request_id OR NEW.message_json!=OLD.message_json OR (OLD.run_id IS NOT NULL AND (NEW.run_id IS NOT NULL OR EXISTS(SELECT 1 FROM model_generation g WHERE g.run_id=OLD.run_id))) BEGIN SELECT RAISE(ABORT,'immutable inbox assignment'); END;
CREATE TRIGGER instruction_no_replace BEFORE INSERT ON task_instruction WHEN EXISTS(SELECT 1 FROM task_instruction WHERE instruction_id=NEW.instruction_id OR sequence=NEW.sequence) BEGIN SELECT RAISE(ABORT,'instruction identity exists'); END;
CREATE TRIGGER task_request_no_delete BEFORE DELETE ON task_request BEGIN SELECT RAISE(ABORT,'task request audit retained'); END;
CREATE TRIGGER task_request_no_replace BEFORE INSERT ON task_request WHEN EXISTS(SELECT 1 FROM task_request WHERE (source_message_id=NEW.source_message_id AND request_hash=NEW.request_hash) OR invocation_id=NEW.invocation_id) BEGIN SELECT RAISE(ABORT,'task request identity exists'); END;
CREATE TRIGGER task_invocation_no_delete BEFORE DELETE ON task_invocation BEGIN SELECT RAISE(ABORT,'task receipt audit retained'); END;
CREATE TRIGGER task_invocation_no_replace BEFORE INSERT ON task_invocation WHEN EXISTS(SELECT 1 FROM task_invocation WHERE invocation_id=NEW.invocation_id) BEGIN SELECT RAISE(ABORT,'task receipt identity exists'); END;
CREATE TRIGGER result_no_delete BEFORE DELETE ON worker_result BEGIN SELECT RAISE(ABORT,'worker result audit retained'); END;
CREATE TRIGGER result_no_replace BEFORE INSERT ON worker_result WHEN EXISTS(SELECT 1 FROM worker_result WHERE exchange_id=NEW.exchange_id) AND (NEW.run_id!=(SELECT run_id FROM worker_result WHERE exchange_id=NEW.exchange_id) OR NEW.content!=(SELECT content FROM worker_result WHERE exchange_id=NEW.exchange_id)) BEGIN SELECT RAISE(ABORT,'worker result identity conflict'); END;
CREATE TRIGGER task_update_no_replace BEFORE INSERT ON task_update WHEN EXISTS(SELECT 1 FROM task_update WHERE event_id=NEW.event_id OR (root_run_id=NEW.root_run_id AND kind=NEW.kind)) BEGIN SELECT RAISE(ABORT,'task update identity exists'); END;
CREATE TRIGGER inbox_no_delete BEFORE DELETE ON main_inbox BEGIN SELECT RAISE(ABORT,'main inbox audit retained'); END;
CREATE TRIGGER task_version BEFORE UPDATE ON task WHEN NEW.version!=OLD.version+1 OR NEW.task_id!=OLD.task_id OR NEW.task_ref!=OLD.task_ref
 OR NEW.agent_id!=OLD.agent_id OR NEW.conversation_id!=OLD.conversation_id OR NEW.title!=OLD.title OR NEW.anchor_task!=OLD.anchor_task
 OR NEW.anchor_artifact_refs!=OLD.anchor_artifact_refs OR NEW.created_at!=OLD.created_at OR NEW.completion_policy!=OLD.completion_policy
 OR NEW.task_epoch!=OLD.task_epoch+CASE WHEN (NEW.status!=OLD.status AND NEW.status IN ('paused','cancelled')) OR (NEW.status='active' AND (OLD.status!='active' OR NEW.task_epoch!=OLD.task_epoch)) THEN 1 ELSE 0 END
 OR (NEW.status!=OLD.status AND NOT ((OLD.status='active' AND NEW.status IN ('paused','completed','cancelled')) OR (OLD.status='paused' AND NEW.status='cancelled') OR (OLD.status IN ('paused','completed','cancelled') AND NEW.status='active')))
BEGIN SELECT RAISE(ABORT,'invalid task transition'); END;
CREATE TRIGGER run_version BEFORE UPDATE ON run WHEN NEW.version!=OLD.version+1 OR OLD.status IN ('completed','failed','cancelled')
 OR NEW.run_id!=OLD.run_id OR NEW.agent_id!=OLD.agent_id OR NEW.kind!=OLD.kind OR NEW.model_snapshot_id!=OLD.model_snapshot_id
 OR NEW.capability_snapshot_id!=OLD.capability_snapshot_id OR NEW.created_at!=OLD.created_at OR NEW.history_boundary!=OLD.history_boundary
 OR NEW.conversation_id!=OLD.conversation_id OR NEW.task_id IS NOT OLD.task_id OR NEW.parent_run_id IS NOT OLD.parent_run_id
 OR NEW.root_run_id IS NOT OLD.root_run_id OR NEW.worker_position IS NOT OLD.worker_position OR NEW.depth!=OLD.depth OR NEW.task_epoch IS NOT OLD.task_epoch
 OR NOT ((NEW.status=OLD.status AND NEW.desired_state!='running' AND NEW.checkpoint_json=OLD.checkpoint_json) OR (OLD.status='queued' AND NEW.status IN ('running','cancelled')) OR (OLD.status='running' AND NEW.status IN ('running','waiting','completed','failed','cancelled')) OR (OLD.status='waiting' AND NEW.status IN ('running','cancelled')))
BEGIN SELECT RAISE(ABORT,'invalid run transition'); END;
CREATE INDEX invocation_wait ON tool_invocation(status,wait_reason,next_wake_at);
CREATE TABLE approval (
 approval_id TEXT PRIMARY KEY, invocation_id TEXT NOT NULL UNIQUE REFERENCES tool_invocation(invocation_id),
 risk_type TEXT NOT NULL, scope TEXT NOT NULL, frozen_arguments_hash TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','approved','denied','expired','invalidated')),
 decision_source TEXT, expires_at INTEGER NOT NULL, decided_at INTEGER,
 created_at INTEGER NOT NULL, version INTEGER NOT NULL CHECK(version>=1)
) STRICT;
CREATE INDEX approval_expiry ON approval(status,expires_at);
CREATE TRIGGER approval_initial BEFORE INSERT ON approval WHEN NEW.status!='pending' OR NEW.version!=1 BEGIN SELECT RAISE(ABORT,'approval starts pending'); END;
CREATE TRIGGER approval_transition BEFORE UPDATE ON approval WHEN NEW.version!=OLD.version+1
 OR NEW.approval_id!=OLD.approval_id OR NEW.invocation_id!=OLD.invocation_id OR NEW.risk_type!=OLD.risk_type OR NEW.scope!=OLD.scope
 OR NEW.frozen_arguments_hash!=OLD.frozen_arguments_hash OR NEW.expires_at!=OLD.expires_at OR NEW.created_at!=OLD.created_at
 OR NOT ((OLD.status='pending' AND NEW.status IN ('approved','denied','expired','invalidated')) OR (OLD.status='approved' AND NEW.status IN ('expired','invalidated')))
BEGIN SELECT RAISE(ABORT,'invalid approval transition'); END;
CREATE TRIGGER approval_no_delete BEFORE DELETE ON approval BEGIN SELECT RAISE(ABORT,'approval audit retained'); END;
CREATE TRIGGER approval_no_replace BEFORE INSERT ON approval WHEN EXISTS(SELECT 1 FROM approval WHERE approval_id=NEW.approval_id) BEGIN SELECT RAISE(ABORT,'approval identity exists'); END;
CREATE TRIGGER invocation_transition BEFORE UPDATE ON tool_invocation WHEN NEW.version!=OLD.version+1 OR OLD.status IN ('succeeded','failed','cancelled')
 OR NEW.arguments_json!=OLD.arguments_json OR NEW.request_hash!=OLD.request_hash OR NEW.replay_policy!=OLD.replay_policy OR NEW.idempotency_key!=OLD.idempotency_key
 OR NEW.provider_call_id!=OLD.provider_call_id OR NEW.provider_exchange_id!=OLD.provider_exchange_id OR NEW.tool_name!=OLD.tool_name OR NEW.run_id!=OLD.run_id
 OR NEW.created_at!=OLD.created_at OR NEW.delivery_seq!=OLD.delivery_seq
 OR NEW.invocation_id!=OLD.invocation_id OR NEW.parent_invocation_id IS NOT OLD.parent_invocation_id OR NEW.policy_json!=OLD.policy_json OR NEW.conflict_keys!=OLD.conflict_keys
 OR (OLD.desired_state='cancelled' AND NEW.desired_state!='cancelled')
 OR NOT ((OLD.status='queued' AND NEW.status IN ('queued','running','waiting','cancelled'))
 OR (OLD.status='running' AND NEW.status IN ('running','waiting','succeeded','failed','cancelled','unknown'))
 OR (OLD.status='waiting' AND NEW.status IN ('waiting','running','cancelled','unknown'))
 OR (OLD.status='unknown' AND NEW.status IN ('succeeded','failed') AND NEW.reconciliation_json IS NOT NULL))
BEGIN SELECT RAISE(ABORT,'invalid tool transition'); END;
CREATE TRIGGER invocation_wait_reason BEFORE UPDATE ON tool_invocation WHEN
 (NEW.status='waiting' AND (NEW.wait_reason IS NULL OR NEW.wait_reason NOT IN ('approval','child_run','external_event','retry_backoff','resource')))
 OR (NEW.status!='waiting' AND NEW.wait_reason IS NOT NULL)
BEGIN SELECT RAISE(ABORT,'invalid tool wait'); END;
CREATE TABLE artifact (
 artifact_id TEXT PRIMARY KEY, artifact_ref TEXT NOT NULL UNIQUE,
 owner_user_id TEXT NOT NULL, owner_agent_id TEXT NOT NULL,
 task_id TEXT REFERENCES task(task_id), run_id TEXT REFERENCES run(run_id),
 media_type TEXT NOT NULL, name TEXT NOT NULL, byte_size INTEGER NOT NULL CHECK(byte_size>=0),
 content_hash TEXT NOT NULL, storage_key TEXT NOT NULL UNIQUE,
 visibility TEXT NOT NULL CHECK(visibility IN ('agent','task','run')),
 retention_class TEXT NOT NULL CHECK(retention_class='standard'),
 created_at INTEGER NOT NULL, expires_at INTEGER NOT NULL, deleted_at INTEGER,
 version INTEGER NOT NULL DEFAULT 1 CHECK(version>=1), put_key TEXT UNIQUE
);
CREATE INDEX artifact_scope ON artifact(owner_user_id,owner_agent_id,task_id,run_id);
CREATE INDEX artifact_expiry ON artifact(expires_at) WHERE deleted_at IS NULL;
CREATE TABLE artifact_reference (
 artifact_ref TEXT NOT NULL REFERENCES artifact(artifact_ref),
 holder_kind TEXT NOT NULL, holder_id TEXT NOT NULL,
 PRIMARY KEY(artifact_ref,holder_kind,holder_id)
);
CREATE TABLE artifact_publication (
 run_id TEXT NOT NULL REFERENCES run(run_id), artifact_ref TEXT NOT NULL REFERENCES artifact(artifact_ref),
 PRIMARY KEY(run_id,artifact_ref)
);
CREATE TRIGGER artifact_immutable BEFORE UPDATE ON artifact WHEN
 NEW.artifact_id IS NOT OLD.artifact_id OR NEW.artifact_ref IS NOT OLD.artifact_ref OR
 NEW.owner_user_id IS NOT OLD.owner_user_id OR NEW.owner_agent_id IS NOT OLD.owner_agent_id OR
 NEW.task_id IS NOT OLD.task_id OR NEW.run_id IS NOT OLD.run_id OR NEW.media_type IS NOT OLD.media_type OR
 NEW.name IS NOT OLD.name OR NEW.byte_size IS NOT OLD.byte_size OR NEW.content_hash IS NOT OLD.content_hash OR
 NEW.storage_key IS NOT OLD.storage_key OR NEW.visibility IS NOT OLD.visibility OR
 NEW.retention_class IS NOT OLD.retention_class OR NEW.created_at IS NOT OLD.created_at OR
 NEW.expires_at IS NOT OLD.expires_at OR NEW.put_key IS NOT OLD.put_key OR
 OLD.deleted_at IS NOT NULL OR NEW.deleted_at IS NULL OR NEW.version!=OLD.version+1
 BEGIN SELECT RAISE(ABORT,'immutable artifact'); END;
CREATE TRIGGER artifact_no_delete BEFORE DELETE ON artifact BEGIN SELECT RAISE(ABORT,'artifact tombstone required'); END;
CREATE TRIGGER artifact_no_replace BEFORE INSERT ON artifact WHEN EXISTS(
 SELECT 1 FROM artifact WHERE artifact_id=NEW.artifact_id OR artifact_ref=NEW.artifact_ref OR storage_key=NEW.storage_key OR put_key=NEW.put_key)
 BEGIN SELECT RAISE(ABORT,'immutable artifact'); END;
CREATE TABLE delivery_request (
 delivery_id TEXT PRIMARY KEY, message_id TEXT NOT NULL UNIQUE,
 conversation_id TEXT NOT NULL, channel TEXT NOT NULL, destination_ref TEXT NOT NULL,
 intent TEXT NOT NULL CHECK(intent IN ('completion','failure','needs_input','approval')),
 status TEXT NOT NULL CHECK(status IN ('pending','sending','retry','sent','failed','unknown')),
 idempotency_key TEXT NOT NULL UNIQUE, attempt_count INTEGER NOT NULL DEFAULT 0 CHECK(attempt_count>=0),
 last_error TEXT, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, version INTEGER NOT NULL CHECK(version>=1)
) STRICT;
CREATE TABLE delivery_outbox (
 delivery_id TEXT PRIMARY KEY REFERENCES delivery_request(delivery_id),
 next_wake_at INTEGER NOT NULL, claim_token TEXT, claim_expires_at INTEGER NOT NULL DEFAULT 0
) STRICT;
CREATE INDEX delivery_outbox_wake ON delivery_outbox(next_wake_at,claim_expires_at);
CREATE TRIGGER delivery_identity BEFORE UPDATE ON delivery_request WHEN
 NEW.version!=OLD.version+1 OR NEW.delivery_id!=OLD.delivery_id OR NEW.message_id!=OLD.message_id OR NEW.conversation_id!=OLD.conversation_id OR
 NEW.channel!=OLD.channel OR NEW.destination_ref!=OLD.destination_ref OR NEW.intent!=OLD.intent OR
 NEW.idempotency_key!=OLD.idempotency_key OR NEW.created_at!=OLD.created_at OR
 NEW.attempt_count<OLD.attempt_count OR OLD.status IN ('sent','failed') OR
 NOT ((OLD.status IN ('pending','retry') AND NEW.status='sending') OR
      (OLD.status='sending' AND NEW.status IN ('sending','sent','retry','failed','unknown')) OR
      (OLD.status='unknown' AND NEW.status IN ('unknown','sent','failed')))
 BEGIN SELECT RAISE(ABORT,'invalid delivery transition'); END;
CREATE TRIGGER delivery_no_delete BEFORE DELETE ON delivery_request BEGIN SELECT RAISE(ABORT,'delivery audit retained'); END;
CREATE TRIGGER delivery_no_replace BEFORE INSERT ON delivery_request WHEN EXISTS(SELECT 1 FROM delivery_request WHERE delivery_id=NEW.delivery_id OR idempotency_key=NEW.idempotency_key OR message_id=NEW.message_id) BEGIN SELECT RAISE(ABORT,'delivery identity exists'); END;
CREATE TRIGGER delivery_initial BEFORE INSERT ON delivery_request WHEN NEW.version!=1 OR NEW.status!='pending' OR NEW.attempt_count!=0 OR NEW.idempotency_key!=NEW.delivery_id BEGIN SELECT RAISE(ABORT,'delivery starts pending with Host key'); END;
CREATE TABLE notification_intent (
 message_id TEXT PRIMARY KEY REFERENCES conversation_intent(message_id),
 intent TEXT NOT NULL CHECK(intent IN ('failure','needs_input','approval'))
) STRICT;
CREATE TRIGGER notification_no_update BEFORE UPDATE ON notification_intent BEGIN SELECT RAISE(ABORT,'immutable notification'); END;
CREATE TRIGGER notification_no_delete BEFORE DELETE ON notification_intent BEGIN SELECT RAISE(ABORT,'notification audit retained'); END;
CREATE TABLE terminal_message (
 idempotency_key TEXT PRIMARY KEY, delivery_id TEXT NOT NULL UNIQUE, message_id TEXT NOT NULL UNIQUE,
 destination_ref TEXT NOT NULL, envelope_json TEXT NOT NULL CHECK(json_valid(envelope_json))
) STRICT;
CREATE TRIGGER terminal_immutable BEFORE UPDATE ON terminal_message BEGIN SELECT RAISE(ABORT,'immutable terminal receipt'); END;
CREATE TRIGGER terminal_retained BEFORE DELETE ON terminal_message BEGIN SELECT RAISE(ABORT,'terminal history retained'); END;
CREATE TRIGGER terminal_no_replace BEFORE INSERT ON terminal_message WHEN EXISTS(SELECT 1 FROM terminal_message WHERE idempotency_key=NEW.idempotency_key OR delivery_id=NEW.delivery_id OR message_id=NEW.message_id) BEGIN SELECT RAISE(ABORT,'terminal identity exists'); END;
CREATE TABLE run_tree (
 root_run_id TEXT PRIMARY KEY REFERENCES run(run_id),
 descendants INTEGER NOT NULL DEFAULT 0 CHECK(descendants>=0),
 tool_calls INTEGER NOT NULL DEFAULT 0 CHECK(tool_calls>=0),
 max_depth INTEGER NOT NULL CHECK(max_depth BETWEEN 1 AND 16),
 max_descendants INTEGER NOT NULL CHECK(max_descendants BETWEEN 1 AND 10000),
 max_tool_calls INTEGER NOT NULL CHECK(max_tool_calls BETWEEN 1 AND 50000),
 version INTEGER NOT NULL CHECK(version>=1)
, max_turns INTEGER NOT NULL DEFAULT 256 CHECK(max_turns BETWEEN 1 AND 512)) STRICT;
CREATE TRIGGER tree_version BEFORE UPDATE ON run_tree WHEN NEW.version!=OLD.version+1 OR NEW.root_run_id!=OLD.root_run_id
 OR NEW.max_depth!=OLD.max_depth OR NEW.max_descendants!=OLD.max_descendants OR NEW.max_tool_calls!=OLD.max_tool_calls
 OR NEW.descendants<OLD.descendants OR NEW.descendants>OLD.descendants+1 OR NEW.tool_calls<OLD.tool_calls OR NEW.tool_calls>OLD.tool_calls+1
BEGIN SELECT RAISE(ABORT,'invalid tree counters'); END;
CREATE TRIGGER tree_no_delete BEFORE DELETE ON run_tree BEGIN SELECT RAISE(ABORT,'tree audit retained'); END;
CREATE TRIGGER tree_no_replace BEFORE INSERT ON run_tree WHEN EXISTS(SELECT 1 FROM run_tree WHERE root_run_id=NEW.root_run_id) BEGIN SELECT RAISE(ABORT,'tree identity exists'); END;
CREATE TABLE agent_invocation (
 invocation_id TEXT PRIMARY KEY REFERENCES tool_invocation(invocation_id),
 child_run_id TEXT NOT NULL UNIQUE REFERENCES run(run_id),
 request_hash TEXT NOT NULL
) STRICT;
CREATE TRIGGER agent_invocation_no_update BEFORE UPDATE ON agent_invocation BEGIN SELECT RAISE(ABORT,'immutable child admission'); END;
CREATE TRIGGER agent_invocation_no_delete BEFORE DELETE ON agent_invocation BEGIN SELECT RAISE(ABORT,'child audit retained'); END;
CREATE TRIGGER agent_invocation_no_replace BEFORE INSERT ON agent_invocation WHEN EXISTS(SELECT 1 FROM agent_invocation WHERE invocation_id=NEW.invocation_id OR child_run_id=NEW.child_run_id) BEGIN SELECT RAISE(ABORT,'child admission exists'); END;
CREATE TRIGGER child_initial BEFORE INSERT ON run WHEN NEW.worker_position='child' AND NOT EXISTS(
 SELECT 1 FROM run p JOIN capability_snapshot pc ON pc.snapshot_id=p.capability_snapshot_id
 JOIN capability_snapshot cc ON cc.snapshot_id=NEW.capability_snapshot_id
 WHERE p.run_id=NEW.parent_run_id AND p.kind='worker' AND p.status='running' AND p.desired_state='running'
 AND p.task_id=NEW.task_id AND p.root_run_id=NEW.root_run_id AND p.task_epoch=NEW.task_epoch
 AND p.agent_id=NEW.agent_id AND p.conversation_id=NEW.conversation_id AND NEW.depth=p.depth+1
 AND p.model_snapshot_id=NEW.model_snapshot_id AND pc.can_spawn_agents=1
 AND cc.role='worker' AND cc.data_scope=pc.data_scope AND cc.agent_config_version=pc.agent_config_version
 AND cc.can_spawn_agents<=pc.can_spawn_agents AND cc.format_version=pc.format_version
 AND NOT EXISTS(SELECT 1 FROM json_each(cc.tools_json) child WHERE NOT EXISTS(SELECT 1 FROM json_each(pc.tools_json) parent WHERE child.value=parent.value))
 AND NOT EXISTS(SELECT 1 FROM json_each(cc.tools_json) child WHERE json_extract(child.value,'$.permission') NOT IN ('read_only','external_effect','artifact_write','spawn_agents') OR json_extract(child.value,'$.name') LIKE 'builtin__task__%')
) BEGIN SELECT RAISE(ABORT,'invalid child inheritance'); END;
CREATE TRIGGER tree_turns_frozen BEFORE UPDATE ON run_tree WHEN NEW.max_turns!=OLD.max_turns
BEGIN SELECT RAISE(ABORT,'immutable tree turn ceiling'); END;
CREATE TABLE scheduler_tree (
 root_run_id TEXT PRIMARY KEY REFERENCES run_tree(root_run_id),
 last_served INTEGER NOT NULL DEFAULT 0 CHECK(last_served>=0)
) STRICT;
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
CREATE TABLE spend_snapshot (
 snapshot_id TEXT PRIMARY KEY REFERENCES model_snapshot(snapshot_id),
 account_key TEXT NOT NULL,
 currency TEXT NOT NULL,
 boundary_micros INTEGER NOT NULL CHECK(boundary_micros>=0),
 price_json TEXT NOT NULL CHECK(json_valid(price_json))
) STRICT;
CREATE TRIGGER spend_snapshot_no_update BEFORE UPDATE ON spend_snapshot BEGIN SELECT RAISE(ABORT,'immutable spend snapshot'); END;
CREATE TRIGGER spend_snapshot_no_delete BEFORE DELETE ON spend_snapshot BEGIN SELECT RAISE(ABORT,'immutable spend snapshot'); END;
CREATE TRIGGER spend_snapshot_no_replace BEFORE INSERT ON spend_snapshot WHEN EXISTS(SELECT 1 FROM spend_snapshot WHERE snapshot_id=NEW.snapshot_id) BEGIN SELECT RAISE(ABORT,'immutable spend snapshot'); END;
CREATE TABLE spend_reservation (
 generation_id TEXT PRIMARY KEY REFERENCES model_generation(generation_id),
 account_key TEXT NOT NULL,
 currency TEXT NOT NULL,
 reserved_micros INTEGER NOT NULL CHECK(reserved_micros>=0),
 actual_micros INTEGER CHECK(actual_micros>=0),
 status TEXT NOT NULL CHECK(status IN ('reserved','priced','usage_missing')),
 CHECK ((status='priced')=(actual_micros IS NOT NULL))
) STRICT;
CREATE INDEX spend_account ON spend_reservation(account_key,currency,status);
CREATE TRIGGER spend_reservation_initial BEFORE INSERT ON spend_reservation WHEN NEW.status!='reserved' OR NEW.actual_micros IS NOT NULL BEGIN SELECT RAISE(ABORT,'spend reservation starts unspent'); END;
CREATE TRIGGER spend_reservation_frozen BEFORE UPDATE ON spend_reservation WHEN NEW.generation_id!=OLD.generation_id OR NEW.account_key!=OLD.account_key OR NEW.currency!=OLD.currency OR NEW.reserved_micros!=OLD.reserved_micros OR OLD.status!='reserved' OR NEW.status NOT IN ('priced','usage_missing') OR (NEW.status='priced')!=(NEW.actual_micros IS NOT NULL)
BEGIN SELECT RAISE(ABORT,'invalid spend settlement'); END;
CREATE TRIGGER spend_reservation_no_delete BEFORE DELETE ON spend_reservation BEGIN SELECT RAISE(ABORT,'spend ledger retained'); END;
CREATE TRIGGER spend_reservation_no_replace BEFORE INSERT ON spend_reservation WHEN EXISTS(SELECT 1 FROM spend_reservation WHERE generation_id=NEW.generation_id) BEGIN SELECT RAISE(ABORT,'spend ledger retained'); END;
CREATE TABLE spend_approval (
 approval_id TEXT PRIMARY KEY,
 root_run_id TEXT NOT NULL UNIQUE REFERENCES run(run_id),
 scope TEXT NOT NULL,
 task_epoch INTEGER,
 arguments_json TEXT NOT NULL CHECK(json_valid(arguments_json)),
 frozen_hash TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','approved','denied','invalidated')),
 created_at INTEGER NOT NULL,
 decided_at INTEGER
) STRICT;
CREATE TRIGGER spend_approval_initial BEFORE INSERT ON spend_approval WHEN NEW.status!='pending' OR NEW.decided_at IS NOT NULL OR json_extract(NEW.arguments_json,'$.batch_run_id') IS NOT NEW.root_run_id OR NOT EXISTS(SELECT 1 FROM run r WHERE r.run_id=NEW.root_run_id AND r.agent_id=NEW.scope AND r.task_epoch IS NEW.task_epoch AND r.desired_state='running' AND r.status IN ('queued','running','waiting') AND (r.kind='main' OR r.worker_position='root'))
BEGIN SELECT RAISE(ABORT,'spend approval requires live batch owner'); END;
CREATE TRIGGER spend_approval_no_replace BEFORE INSERT ON spend_approval WHEN EXISTS(SELECT 1 FROM spend_approval WHERE approval_id=NEW.approval_id OR root_run_id=NEW.root_run_id) BEGIN SELECT RAISE(ABORT,'spend approval retained'); END;
CREATE TRIGGER spend_approval_frozen BEFORE UPDATE ON spend_approval WHEN NEW.approval_id!=OLD.approval_id OR NEW.root_run_id!=OLD.root_run_id OR NEW.scope!=OLD.scope OR NEW.task_epoch IS NOT OLD.task_epoch OR NEW.arguments_json!=OLD.arguments_json OR NEW.frozen_hash!=OLD.frozen_hash OR NEW.created_at!=OLD.created_at OR OLD.status!='pending' OR NEW.status='pending'
BEGIN SELECT RAISE(ABORT,'invalid spend approval transition'); END;
CREATE TRIGGER spend_approval_no_delete BEFORE DELETE ON spend_approval BEGIN SELECT RAISE(ABORT,'spend approval retained'); END;
CREATE VIEW spend_unpriced_usage AS SELECT u.generation_id,u.snapshot_id,
 json_extract(u.snapshot_json,'$.provider_kind')||':'||coalesce(nullif(json_extract(u.snapshot_json,'$.auth_connection_id'),''),json_extract(u.snapshot_json,'$.endpoint_hash'),'') AS account_key
 FROM provider_usage u LEFT JOIN spend_snapshot s USING(snapshot_id)
 WHERE s.snapshot_id IS NULL OR s.price_json='null';
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
 version INTEGER NOT NULL CHECK(version>=1), value_hash TEXT NOT NULL DEFAULT '', updated_at INTEGER NOT NULL DEFAULT 0,
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
 request_hash TEXT NOT NULL, call_key TEXT GENERATED ALWAYS AS (substr(call_id,length(execution_id)+2)) VIRTUAL,
 PRIMARY KEY(execution_id,call_id)
) STRICT;
CREATE TRIGGER code_call_immutable BEFORE UPDATE ON code_call BEGIN SELECT RAISE(ABORT,'immutable code call'); END;
CREATE TRIGGER code_call_retained BEFORE DELETE ON code_call BEGIN SELECT RAISE(ABORT,'code call retained'); END;
CREATE TABLE code_outcome (
 execution_id TEXT PRIMARY KEY REFERENCES code_execution(execution_id),
 outcome_json TEXT NOT NULL CHECK(json_valid(outcome_json)),
 created_at INTEGER NOT NULL
) STRICT;
CREATE TRIGGER code_outcome_immutable BEFORE UPDATE ON code_outcome BEGIN SELECT RAISE(ABORT,'immutable cell outcome'); END;
CREATE TRIGGER code_outcome_retained BEFORE DELETE ON code_outcome BEGIN SELECT RAISE(ABORT,'cell outcome retained'); END;
CREATE TABLE code_manifest (
 execution_id TEXT PRIMARY KEY REFERENCES code_execution(execution_id),
 source TEXT NOT NULL, input_json TEXT NOT NULL CHECK(json_valid(input_json)),
 input_hash TEXT NOT NULL, snapshot_hash TEXT NOT NULL, manifest_hash TEXT NOT NULL,
 request_json TEXT NOT NULL CHECK(json_valid(request_json))
) STRICT;
CREATE TRIGGER code_manifest_immutable BEFORE UPDATE ON code_manifest BEGIN SELECT RAISE(ABORT,'immutable code manifest'); END;
CREATE TRIGGER code_manifest_retained BEFORE DELETE ON code_manifest BEGIN SELECT RAISE(ABORT,'code manifest retained'); END;
CREATE TRIGGER code_manifest_replace BEFORE INSERT ON code_manifest WHEN EXISTS(SELECT 1 FROM code_manifest WHERE execution_id=NEW.execution_id) BEGIN SELECT RAISE(ABORT,'code manifest exists'); END;
CREATE TABLE code_execution_state (
 execution_id TEXT PRIMARY KEY REFERENCES code_execution(execution_id),
 state TEXT NOT NULL DEFAULT 'suspended' CHECK(state IN ('running','suspended','completed','failed','cancelled')),
 desired_state TEXT NOT NULL DEFAULT 'running' CHECK(desired_state IN ('running','paused','cancelled')),
 wake_seq INTEGER NOT NULL DEFAULT 0, processed_wake_seq INTEGER NOT NULL DEFAULT 0,
 execution_epoch INTEGER NOT NULL DEFAULT 0, claim_token TEXT, owner_run_token TEXT,
 claim_expires_at INTEGER NOT NULL DEFAULT 0, result_ref TEXT, error_json TEXT,
 replay_starts INTEGER NOT NULL DEFAULT 0, replay_hits INTEGER NOT NULL DEFAULT 0,
 created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, version INTEGER NOT NULL DEFAULT 1,
 CHECK(wake_seq>=processed_wake_seq AND processed_wake_seq>=0)
) STRICT;
CREATE INDEX code_recovery ON code_execution_state(state,claim_expires_at,wake_seq,processed_wake_seq);
CREATE TRIGGER code_state_version BEFORE UPDATE ON code_execution_state WHEN NEW.version!=OLD.version+1 OR NEW.execution_id!=OLD.execution_id OR NEW.created_at!=OLD.created_at OR NEW.wake_seq<OLD.wake_seq OR NEW.processed_wake_seq<OLD.processed_wake_seq OR NEW.execution_epoch<OLD.execution_epoch BEGIN SELECT RAISE(ABORT,'invalid code state update'); END;
CREATE VIEW code_call_state AS SELECT c.*,i.tool_name,i.status,i.result_ref,i.error_json,i.attempt_count,i.created_at,i.updated_at,a.child_run_id FROM code_call c JOIN tool_invocation i USING(invocation_id) LEFT JOIN agent_invocation a USING(invocation_id);
CREATE TABLE code_delivery (
 execution_id TEXT NOT NULL REFERENCES code_execution(execution_id), call_id TEXT NOT NULL,
 sequence INTEGER NOT NULL CHECK(sequence>0), reply_json TEXT NOT NULL CHECK(json_valid(reply_json)),
 PRIMARY KEY(execution_id,call_id), UNIQUE(execution_id,sequence)
) STRICT;
CREATE TRIGGER code_delivery_immutable BEFORE UPDATE ON code_delivery BEGIN SELECT RAISE(ABORT,'immutable code delivery'); END;
CREATE TRIGGER code_delivery_retained BEFORE DELETE ON code_delivery BEGIN SELECT RAISE(ABORT,'code delivery retained'); END;
CREATE TRIGGER code_tool_wake AFTER UPDATE OF status ON tool_invocation WHEN NEW.parent_invocation_id IS NOT NULL AND NEW.status!=OLD.status BEGIN
 UPDATE code_execution_state SET wake_seq=wake_seq+1,updated_at=NEW.updated_at,version=version+1 WHERE execution_id=NEW.parent_invocation_id;
END;
CREATE TRIGGER code_approval_wake AFTER UPDATE OF status ON approval BEGIN
 UPDATE code_execution_state SET wake_seq=wake_seq+1,version=version+1 WHERE execution_id=(SELECT parent_invocation_id FROM tool_invocation WHERE invocation_id=NEW.invocation_id);
END;
CREATE TRIGGER code_child_wake AFTER UPDATE OF status ON run WHEN NEW.status!=OLD.status BEGIN
 UPDATE code_execution_state SET wake_seq=wake_seq+1,updated_at=NEW.updated_at,version=version+1 WHERE execution_id IN (SELECT i.parent_invocation_id FROM agent_invocation a JOIN tool_invocation i USING(invocation_id) WHERE a.child_run_id=NEW.run_id);
END;
CREATE TRIGGER code_stop_intent AFTER UPDATE OF desired_state ON run WHEN NEW.desired_state!=OLD.desired_state BEGIN
 UPDATE code_execution_state SET desired_state=CASE WHEN NEW.desired_state='running' THEN 'running' WHEN NEW.desired_state='task_paused' THEN 'paused' ELSE 'cancelled' END,wake_seq=wake_seq+1,claim_token=NULL,owner_run_token=NULL,claim_expires_at=0,version=version+1 WHERE execution_id IN (SELECT invocation_id FROM tool_invocation WHERE run_id=NEW.run_id) AND state IN ('running','suspended');
END;
CREATE TRIGGER code_wrapper_terminal AFTER UPDATE OF status ON tool_invocation WHEN NEW.tool_name='builtin__codemode__execute' AND NEW.status IN ('succeeded','failed','cancelled','unknown') BEGIN
 UPDATE code_execution_state SET state=CASE WHEN NEW.status='succeeded' THEN 'completed' WHEN NEW.status='cancelled' THEN 'cancelled' ELSE 'failed' END,processed_wake_seq=wake_seq,result_ref=NEW.result_ref,error_json=NEW.error_json,claim_token=NULL,owner_run_token=NULL,claim_expires_at=0,updated_at=NEW.updated_at,version=version+1 WHERE execution_id=NEW.invocation_id;
END;
CREATE UNIQUE INDEX code_call_key ON code_call(execution_id,call_key);
CREATE TRIGGER code_instruction_wake AFTER INSERT ON task_instruction BEGIN
 UPDATE code_execution_state SET wake_seq=wake_seq+1,version=version+1 WHERE execution_id IN (SELECT i.invocation_id FROM tool_invocation i JOIN run r USING(run_id) WHERE r.task_id=NEW.task_id AND r.task_epoch=NEW.task_epoch) AND state IN ('running','suspended');
END;
CREATE TRIGGER code_input_wake AFTER INSERT ON main_input BEGIN
 UPDATE code_execution_state SET wake_seq=wake_seq+1,version=version+1 WHERE execution_id IN (SELECT invocation_id FROM tool_invocation WHERE run_id=NEW.run_id) AND state IN ('running','suspended');
END;
CREATE TRIGGER code_spend_wake AFTER UPDATE OF status ON spend_approval BEGIN
 UPDATE code_execution_state SET wake_seq=wake_seq+1,version=version+1 WHERE execution_id IN (SELECT i.invocation_id FROM tool_invocation i JOIN run r USING(run_id) WHERE coalesce(r.root_run_id,r.run_id)=NEW.root_run_id) AND state IN ('running','suspended');
END;
CREATE TABLE code_store_call (
 execution_id TEXT NOT NULL REFERENCES code_execution(execution_id), call_key TEXT NOT NULL,
 request_hash TEXT NOT NULL, reply_json TEXT NOT NULL CHECK(json_valid(reply_json)),
 PRIMARY KEY(execution_id,call_key)
) STRICT;
CREATE TRIGGER code_store_call_immutable BEFORE UPDATE ON code_store_call BEGIN SELECT RAISE(ABORT,'immutable store call'); END;
CREATE TRIGGER code_store_call_retained BEFORE DELETE ON code_store_call BEGIN SELECT RAISE(ABORT,'store call retained'); END;
CREATE TABLE run_recall (
 run_id TEXT NOT NULL REFERENCES run(run_id), step_index INTEGER NOT NULL,
 embedding_id TEXT NOT NULL, query TEXT NOT NULL, hits_json TEXT NOT NULL CHECK(json_valid(hits_json)),
 created_at INTEGER NOT NULL, PRIMARY KEY(run_id,step_index)
) STRICT;
CREATE TRIGGER run_recall_immutable BEFORE UPDATE ON run_recall BEGIN SELECT RAISE(ABORT,'immutable recall'); END;
CREATE TRIGGER run_recall_no_delete BEFORE DELETE ON run_recall BEGIN SELECT RAISE(ABORT,'immutable recall'); END;
CREATE TRIGGER run_recall_no_replace BEFORE INSERT ON run_recall WHEN EXISTS(SELECT 1 FROM run_recall WHERE run_id=new.run_id AND step_index=new.step_index) BEGIN SELECT RAISE(ABORT,'immutable recall'); END;
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
CREATE TABLE legacy_context_generation(generation_id TEXT PRIMARY KEY REFERENCES model_generation(generation_id)) STRICT;
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
CREATE UNIQUE INDEX one_main ON run(agent_id)
 WHERE kind='main' AND desired_state='running' AND status IN ('queued','running','waiting');
