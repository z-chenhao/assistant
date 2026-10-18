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
