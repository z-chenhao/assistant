-- Prices/policy freeze with the model snapshot. Existing snapshots explicitly
-- remain unconfigured; never retroactively guess prices for historical usage.
CREATE TABLE spend_snapshot (
 snapshot_id TEXT PRIMARY KEY REFERENCES model_snapshot(snapshot_id),
 account_key TEXT NOT NULL,
 currency TEXT NOT NULL,
 boundary_micros INTEGER NOT NULL CHECK(boundary_micros>=0),
 price_json TEXT NOT NULL CHECK(json_valid(price_json))
) STRICT;
INSERT INTO spend_snapshot SELECT snapshot_id,provider_kind||':'||coalesce(auth_connection_id,endpoint_hash,''),'',0,'null' FROM model_snapshot;
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
-- Probe snapshots have usage JSON but no business Run/ModelSnapshot row.
-- Show them as unpriced observations rather than silently omitting them.
CREATE VIEW spend_unpriced_usage AS SELECT u.generation_id,u.snapshot_id,
 json_extract(u.snapshot_json,'$.provider_kind')||':'||coalesce(nullif(json_extract(u.snapshot_json,'$.auth_connection_id'),''),json_extract(u.snapshot_json,'$.endpoint_hash'),'') AS account_key
 FROM provider_usage u LEFT JOIN spend_snapshot s USING(snapshot_id)
 WHERE s.snapshot_id IS NULL OR s.price_json='null';
