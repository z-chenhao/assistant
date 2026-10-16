-- Preserve existing intents and receipts; local_cli was the previous name
-- for the single local terminal destination. Sent receipts are never replayed.
ALTER TABLE delivery_request RENAME TO delivery_request_v11;
CREATE TABLE delivery_request (
 delivery_id TEXT PRIMARY KEY, message_id TEXT NOT NULL UNIQUE,
 conversation_id TEXT NOT NULL, channel TEXT NOT NULL, destination_ref TEXT NOT NULL,
 intent TEXT NOT NULL CHECK(intent IN ('completion','failure','needs_input','approval')),
 status TEXT NOT NULL CHECK(status IN ('pending','sending','retry','sent','failed','unknown')),
 idempotency_key TEXT NOT NULL UNIQUE, attempt_count INTEGER NOT NULL DEFAULT 0 CHECK(attempt_count>=0),
 last_error TEXT, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, version INTEGER NOT NULL CHECK(version>=1)
) STRICT;
INSERT INTO delivery_request SELECT delivery_id,message_id,destination_ref,
 CASE channel WHEN 'local_cli' THEN 'tui' ELSE channel END,destination_ref,'completion',status,idempotency_key,attempt_count,last_error,created_at,updated_at,1 FROM delivery_request_v11;
DROP TABLE delivery_request_v11;
CREATE TABLE delivery_outbox (
 delivery_id TEXT PRIMARY KEY REFERENCES delivery_request(delivery_id),
 next_wake_at INTEGER NOT NULL, claim_token TEXT, claim_expires_at INTEGER NOT NULL DEFAULT 0
) STRICT;
INSERT INTO delivery_outbox(delivery_id,next_wake_at) SELECT delivery_id,created_at FROM delivery_request WHERE status='pending';
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
-- Security and needs-input/failure notices use the same recoverable
-- Conversation-first commit order, without another inference round.
CREATE TABLE notification_intent (
 message_id TEXT PRIMARY KEY REFERENCES conversation_intent(message_id),
 intent TEXT NOT NULL CHECK(intent IN ('failure','needs_input','approval'))
) STRICT;
CREATE TRIGGER notification_no_update BEFORE UPDATE ON notification_intent BEGIN SELECT RAISE(ABORT,'immutable notification'); END;
CREATE TRIGGER notification_no_delete BEFORE DELETE ON notification_intent BEGIN SELECT RAISE(ABORT,'notification audit retained'); END;

-- The terminal is a Host-owned channel projection. Acceptance commits before
-- the DeliveryRequest acknowledgement, using a distinct transaction in state.db.
CREATE TABLE terminal_message (
 idempotency_key TEXT PRIMARY KEY, delivery_id TEXT NOT NULL UNIQUE, message_id TEXT NOT NULL UNIQUE,
 destination_ref TEXT NOT NULL, envelope_json TEXT NOT NULL CHECK(json_valid(envelope_json))
) STRICT;
CREATE TRIGGER terminal_immutable BEFORE UPDATE ON terminal_message BEGIN SELECT RAISE(ABORT,'immutable terminal receipt'); END;
CREATE TRIGGER terminal_retained BEFORE DELETE ON terminal_message BEGIN SELECT RAISE(ABORT,'terminal history retained'); END;
CREATE TRIGGER terminal_no_replace BEFORE INSERT ON terminal_message WHEN EXISTS(SELECT 1 FROM terminal_message WHERE idempotency_key=NEW.idempotency_key OR delivery_id=NEW.delivery_id OR message_id=NEW.message_id) BEGIN SELECT RAISE(ABORT,'terminal identity exists'); END;
