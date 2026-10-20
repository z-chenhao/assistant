-- Epoch zero means no accepted Forget. Legacy intent has no reliable acceptance
-- provenance: retain its audit unchanged and fail closed if a barrier exists.
ALTER TABLE automation ADD COLUMN intent_privacy_epoch INTEGER NOT NULL DEFAULT 0 CHECK(intent_privacy_epoch>=0);
CREATE TRIGGER automation_intent_privacy_immutable BEFORE UPDATE ON automation
 WHEN NEW.intent_privacy_epoch!=OLD.intent_privacy_epoch
 BEGIN SELECT RAISE(ABORT,'immutable automation intent privacy'); END;
