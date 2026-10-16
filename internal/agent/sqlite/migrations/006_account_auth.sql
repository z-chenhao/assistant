-- Installation identity is deliberately absent from diagnostics/events.
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
