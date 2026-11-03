-- Old accepted routes intentionally remain unthreaded. New reply sources must
-- already belong to the same explicitly enrolled personal binding.
ALTER TABLE message_route ADD COLUMN reply_source_id TEXT NOT NULL DEFAULT '' CHECK(length(reply_source_id)<=256);
CREATE TRIGGER message_reply_source BEFORE INSERT ON message_route
 WHEN NEW.reply_source_id!='' AND NOT EXISTS(
  SELECT 1 FROM channel_source s JOIN channel_binding b USING(binding_id)
  WHERE s.binding_id=NEW.binding_id AND s.source_id=NEW.reply_source_id AND b.channel='feishu'
 ) BEGIN SELECT RAISE(ABORT,'reply source is not bound'); END;
