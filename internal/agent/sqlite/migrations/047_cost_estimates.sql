-- Catalog estimates are immutable observations, independent of monetary consent.
-- Historical calls deliberately remain unpriced.
CREATE TABLE generation_cost_estimate (
 generation_id TEXT PRIMARY KEY REFERENCES model_generation(generation_id),
 currency TEXT NOT NULL CHECK(length(currency)=3),
 estimated_micros INTEGER NOT NULL CHECK(estimated_micros>=0),
 price_json TEXT NOT NULL CHECK(json_valid(price_json)),
 source TEXT NOT NULL CHECK(length(source)>0)
) STRICT;
CREATE TRIGGER generation_cost_estimate_no_replace BEFORE INSERT ON generation_cost_estimate WHEN EXISTS(SELECT 1 FROM generation_cost_estimate WHERE generation_id=NEW.generation_id) BEGIN SELECT RAISE(ABORT,'estimate retained'); END;
CREATE TRIGGER generation_cost_estimate_no_update BEFORE UPDATE ON generation_cost_estimate BEGIN SELECT RAISE(ABORT,'immutable estimate'); END;
CREATE TRIGGER generation_cost_estimate_no_delete BEFORE DELETE ON generation_cost_estimate BEGIN SELECT RAISE(ABORT,'estimate retained'); END;
