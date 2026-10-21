-- Frozen execution metadata is part of admission; prior accepted pure calls
-- retain the exact original capability/tool schema and fixed policy defaults.
ALTER TABLE capability_snapshot ADD COLUMN format_version INTEGER NOT NULL DEFAULT 1 CHECK(format_version IN (1,2));
ALTER TABLE tool_invocation ADD COLUMN policy_json TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(policy_json));
ALTER TABLE tool_invocation ADD COLUMN conflict_keys TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(conflict_keys));
ALTER TABLE tool_invocation ADD COLUMN next_wake_at INTEGER;
ALTER TABLE tool_invocation ADD COLUMN execution_token TEXT;
ALTER TABLE tool_invocation ADD COLUMN reconciliation_json TEXT CHECK(reconciliation_json IS NULL OR json_valid(reconciliation_json));
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

DROP TRIGGER invocation_transition;
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
