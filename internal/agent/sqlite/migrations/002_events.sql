-- Internal event intent is authoritative in state.db; broker delivery is at least once.
CREATE TABLE event_outbox (
    outbox_id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL UNIQUE,
    subject TEXT NOT NULL CHECK (length(subject) > 0),
    type TEXT NOT NULL CHECK (length(type) > 0),
    payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
    occurred_at INTEGER NOT NULL,
    published_at INTEGER,
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0)
) STRICT;
CREATE INDEX event_outbox_pending ON event_outbox(outbox_id) WHERE published_at IS NULL;
CREATE TABLE event_inbox (
    consumer TEXT NOT NULL CHECK (length(consumer) > 0),
    event_id TEXT NOT NULL,
    processed_at INTEGER NOT NULL,
    PRIMARY KEY (consumer, event_id)
) STRICT;
