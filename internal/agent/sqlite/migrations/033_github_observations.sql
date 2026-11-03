-- Operator-authorized, exact-PR observation. No Task or Run is created here.
CREATE TABLE github_watch (
 scope TEXT PRIMARY KEY, repository TEXT NOT NULL, pull_number INTEGER NOT NULL CHECK(pull_number>0),
 repository_id INTEGER, enabled INTEGER NOT NULL CHECK(enabled IN (0,1)),
 intent_privacy_epoch INTEGER NOT NULL CHECK(intent_privacy_epoch>=0),
 state TEXT NOT NULL, failure_code TEXT NOT NULL DEFAULT '', head_sha TEXT NOT NULL DEFAULT '',
 last_attempt_at INTEGER, last_success_at INTEGER, version INTEGER NOT NULL CHECK(version>=1)
) STRICT;
CREATE TRIGGER github_watch_version BEFORE UPDATE ON github_watch WHEN
 NEW.version!=OLD.version+1 OR NEW.scope!=OLD.scope OR NEW.repository!=OLD.repository OR NEW.pull_number!=OLD.pull_number
 BEGIN SELECT RAISE(ABORT,'invalid GitHub watch change'); END;
CREATE TRIGGER github_watch_retained BEFORE DELETE ON github_watch BEGIN SELECT RAISE(ABORT,'GitHub watch retained'); END;
CREATE TABLE github_observation (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT, scope TEXT NOT NULL REFERENCES github_watch(scope),
 event_key TEXT NOT NULL, kind TEXT NOT NULL, source_id INTEGER NOT NULL,
 content_json TEXT NOT NULL CHECK(json_valid(content_json)), received_at INTEGER NOT NULL,
 intent_privacy_epoch INTEGER NOT NULL CHECK(intent_privacy_epoch>=0), UNIQUE(scope,event_key)
) STRICT;
CREATE INDEX github_observation_scope ON github_observation(scope,sequence);
CREATE TRIGGER github_observation_immutable BEFORE UPDATE ON github_observation BEGIN SELECT RAISE(ABORT,'immutable GitHub observation'); END;
CREATE TRIGGER github_observation_retained BEFORE DELETE ON github_observation BEGIN SELECT RAISE(ABORT,'GitHub observation retained'); END;
