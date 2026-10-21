-- Bindings are created only by the authenticated installation owner. A callback
-- cannot enroll an identity or alter the destination of an accepted delivery.
CREATE TABLE channel_binding (
 binding_id TEXT PRIMARY KEY, channel TEXT NOT NULL CHECK(channel IN ('web','feishu')),
 subject TEXT NOT NULL, destination TEXT NOT NULL, enabled INTEGER NOT NULL CHECK(enabled IN (0,1)),
 version INTEGER NOT NULL CHECK(version>=1), UNIQUE(channel,subject,destination)
) STRICT;
CREATE TRIGGER channel_binding_transition BEFORE UPDATE ON channel_binding WHEN
 NEW.version!=OLD.version+1 OR NEW.binding_id!=OLD.binding_id OR NEW.channel!=OLD.channel OR NEW.subject!=OLD.subject OR NEW.destination!=OLD.destination
 BEGIN SELECT RAISE(ABORT,'invalid binding change'); END;
CREATE TRIGGER channel_binding_retained BEFORE DELETE ON channel_binding BEGIN SELECT RAISE(ABORT,'binding retained'); END;
CREATE TABLE channel_source (
 binding_id TEXT NOT NULL REFERENCES channel_binding(binding_id), source_id TEXT NOT NULL,
 request_key TEXT NOT NULL UNIQUE, content_hash TEXT NOT NULL,
 PRIMARY KEY(binding_id,source_id)
) STRICT;
CREATE TABLE channel_callback (
 channel TEXT NOT NULL, event_id TEXT NOT NULL, binding_id TEXT NOT NULL REFERENCES channel_binding(binding_id),
 source_id TEXT NOT NULL, content_hash TEXT NOT NULL, PRIMARY KEY(channel,event_id),
 FOREIGN KEY(binding_id,source_id) REFERENCES channel_source(binding_id,source_id)
) STRICT;
CREATE TABLE message_route (
 message_id TEXT PRIMARY KEY REFERENCES conversation_intent(message_id), binding_id TEXT NOT NULL REFERENCES channel_binding(binding_id)
) STRICT;
-- Mutable user preference affects only future messages. Feishu is preferred
-- when enrolled; a fresh installation still works without any remote channel.
CREATE TABLE channel_preference (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1), channel TEXT NOT NULL CHECK(channel IN ('auto','tui','web','feishu')),
 binding_id TEXT REFERENCES channel_binding(binding_id), version INTEGER NOT NULL CHECK(version>=1)
) STRICT;
INSERT INTO channel_preference VALUES(1,'auto',NULL,1);
CREATE TRIGGER channel_preference_version BEFORE UPDATE ON channel_preference WHEN NEW.version!=OLD.version+1 BEGIN SELECT RAISE(ABORT,'preference CAS required'); END;
CREATE TABLE delivery_projection (
 delivery_id TEXT PRIMARY KEY REFERENCES delivery_request(delivery_id), envelope_hash TEXT NOT NULL,
 schema_version INTEGER NOT NULL, spec_json TEXT NOT NULL CHECK(json_valid(spec_json))
) STRICT;
CREATE TABLE channel_receipt (
 delivery_id TEXT NOT NULL REFERENCES delivery_request(delivery_id), part INTEGER NOT NULL,
 payload_hash TEXT NOT NULL, remote_id TEXT NOT NULL, PRIMARY KEY(delivery_id,part)
) STRICT;
CREATE TRIGGER channel_source_immutable BEFORE UPDATE ON channel_source BEGIN SELECT RAISE(ABORT,'immutable source'); END;
CREATE TRIGGER channel_source_retained BEFORE DELETE ON channel_source BEGIN SELECT RAISE(ABORT,'source retained'); END;
CREATE TRIGGER channel_callback_immutable BEFORE UPDATE ON channel_callback BEGIN SELECT RAISE(ABORT,'immutable callback'); END;
CREATE TRIGGER channel_callback_retained BEFORE DELETE ON channel_callback BEGIN SELECT RAISE(ABORT,'callback retained'); END;
CREATE TRIGGER message_route_immutable BEFORE UPDATE ON message_route BEGIN SELECT RAISE(ABORT,'immutable route'); END;
CREATE TRIGGER message_route_retained BEFORE DELETE ON message_route BEGIN SELECT RAISE(ABORT,'route retained'); END;
CREATE TRIGGER delivery_projection_immutable BEFORE UPDATE ON delivery_projection BEGIN SELECT RAISE(ABORT,'immutable projection'); END;
CREATE TRIGGER delivery_projection_retained BEFORE DELETE ON delivery_projection BEGIN SELECT RAISE(ABORT,'projection retained'); END;
CREATE TRIGGER channel_receipt_immutable BEFORE UPDATE ON channel_receipt BEGIN SELECT RAISE(ABORT,'immutable receipt'); END;
CREATE TRIGGER channel_receipt_retained BEFORE DELETE ON channel_receipt BEGIN SELECT RAISE(ABORT,'receipt retained'); END;

CREATE TABLE channel_resource (
 binding_id TEXT NOT NULL REFERENCES channel_binding(binding_id), source_id TEXT NOT NULL,
 resource_key TEXT NOT NULL, artifact_ref TEXT NOT NULL, PRIMARY KEY(binding_id,source_id,resource_key)
) STRICT;
CREATE TRIGGER channel_resource_immutable BEFORE UPDATE ON channel_resource BEGIN SELECT RAISE(ABORT,'immutable resource'); END;
CREATE TRIGGER channel_resource_retained BEFORE DELETE ON channel_resource BEGIN SELECT RAISE(ABORT,'resource retained'); END;
CREATE TRIGGER channel_binding_no_replace BEFORE INSERT ON channel_binding WHEN EXISTS(SELECT 1 FROM channel_binding WHERE binding_id=NEW.binding_id OR (channel=NEW.channel AND subject=NEW.subject AND destination=NEW.destination)) BEGIN SELECT RAISE(ABORT,'channel identity retained'); END;
CREATE TRIGGER channel_source_no_replace BEFORE INSERT ON channel_source WHEN EXISTS(SELECT 1 FROM channel_source WHERE request_key=NEW.request_key OR (binding_id=NEW.binding_id AND source_id=NEW.source_id)) BEGIN SELECT RAISE(ABORT,'channel identity retained'); END;
CREATE TRIGGER channel_callback_no_replace BEFORE INSERT ON channel_callback WHEN EXISTS(SELECT 1 FROM channel_callback WHERE channel=NEW.channel AND event_id=NEW.event_id) BEGIN SELECT RAISE(ABORT,'channel identity retained'); END;
CREATE TRIGGER message_route_no_replace BEFORE INSERT ON message_route WHEN EXISTS(SELECT 1 FROM message_route WHERE message_id=NEW.message_id) BEGIN SELECT RAISE(ABORT,'channel identity retained'); END;
CREATE TRIGGER delivery_projection_no_replace BEFORE INSERT ON delivery_projection WHEN EXISTS(SELECT 1 FROM delivery_projection WHERE delivery_id=NEW.delivery_id) BEGIN SELECT RAISE(ABORT,'channel identity retained'); END;
CREATE TRIGGER channel_receipt_no_replace BEFORE INSERT ON channel_receipt WHEN EXISTS(SELECT 1 FROM channel_receipt WHERE delivery_id=NEW.delivery_id AND part=NEW.part) BEGIN SELECT RAISE(ABORT,'channel identity retained'); END;
CREATE TRIGGER channel_resource_no_replace BEFORE INSERT ON channel_resource WHEN EXISTS(SELECT 1 FROM channel_resource WHERE binding_id=NEW.binding_id AND source_id=NEW.source_id AND resource_key=NEW.resource_key) BEGIN SELECT RAISE(ABORT,'channel identity retained'); END;
CREATE TRIGGER channel_preference_no_replace BEFORE INSERT ON channel_preference WHEN EXISTS(SELECT 1 FROM channel_preference WHERE singleton=NEW.singleton) BEGIN SELECT RAISE(ABORT,'channel identity retained'); END;
CREATE TRIGGER channel_preference_retained BEFORE DELETE ON channel_preference BEGIN SELECT RAISE(ABORT,'preference retained'); END;
