-- Internal publication failures are isolated from healthy aggregate wakeups.
ALTER TABLE event_outbox ADD COLUMN dead_at INTEGER;
ALTER TABLE event_outbox ADD COLUMN failure_code TEXT;
ALTER TABLE event_outbox ADD COLUMN failure_count INTEGER NOT NULL DEFAULT 0 CHECK(failure_count>=0);
CREATE INDEX event_outbox_live ON event_outbox(attempt_count,outbox_id) WHERE published_at IS NULL AND dead_at IS NULL;
