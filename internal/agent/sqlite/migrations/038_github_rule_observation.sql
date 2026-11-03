-- Existing explicit bindings keep their unfiltered automatic behavior.
ALTER TABLE github_task_binding ADD COLUMN rule_mode TEXT NOT NULL DEFAULT 'automatic'
 CHECK(rule_mode IN ('observe','automatic'));
ALTER TABLE github_task_binding ADD COLUMN rule_event_kind TEXT NOT NULL DEFAULT ''
 CHECK(rule_event_kind IN ('','review_comment','review','check_run') AND (rule_mode='automatic' OR rule_event_kind!=''));
-- NULL identifies pre-migration observations. New scans record their binding
-- generation (zero when unbound) even if the scan ends under a later rule.
ALTER TABLE github_observation ADD COLUMN binding_generation INTEGER CHECK(binding_generation>=0);

CREATE TABLE github_rule_decision (
 observation_sequence INTEGER PRIMARY KEY REFERENCES github_observation(sequence),
 task_id TEXT NOT NULL REFERENCES task(task_id),
 mode TEXT NOT NULL CHECK(mode IN ('observe','automatic')),
 event_kind TEXT NOT NULL CHECK(event_kind IN ('review_comment','review','check_run')),
 matched INTEGER NOT NULL CHECK(matched IN (0,1))
) STRICT;
CREATE TRIGGER github_rule_decision_immutable BEFORE UPDATE ON github_rule_decision
 BEGIN SELECT RAISE(ABORT,'immutable GitHub rule decision'); END;
CREATE TRIGGER github_rule_decision_retained BEFORE DELETE ON github_rule_decision
 BEGIN SELECT RAISE(ABORT,'GitHub rule decision retained'); END;
