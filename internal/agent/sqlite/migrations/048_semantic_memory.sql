-- Private maintenance generations have no user Task/Run or public tools.
-- Requests/results are Vault sealed; model snapshots contain metadata only.
CREATE TABLE memory_generation (
 generation_id TEXT PRIMARY KEY,event_id TEXT NOT NULL REFERENCES memory_mutation(event_id),
 agent_id TEXT NOT NULL,privacy_epoch INTEGER NOT NULL,model_snapshot_id TEXT NOT NULL REFERENCES model_snapshot(snapshot_id),
 input_hash TEXT NOT NULL,request_sealed TEXT NOT NULL,result_sealed TEXT,
 status TEXT NOT NULL CHECK(status IN('pending','completed','failed')),
 owner_token TEXT NOT NULL,lease_until INTEGER NOT NULL,created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX memory_generation_event ON memory_generation(event_id,created_at);
CREATE TABLE memory_semantic_receipt(event_id TEXT PRIMARY KEY REFERENCES memory_mutation(event_id),completed_at INTEGER NOT NULL) STRICT;
CREATE TABLE memory_cost_reservation (
 generation_id TEXT PRIMARY KEY REFERENCES memory_generation(generation_id),account_key TEXT NOT NULL,currency TEXT NOT NULL,
 reserved_micros INTEGER NOT NULL,actual_micros INTEGER,status TEXT NOT NULL CHECK(status IN('reserved','priced','usage_missing'))
) STRICT;
