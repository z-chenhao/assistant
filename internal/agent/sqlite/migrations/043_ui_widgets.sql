CREATE TABLE ui_widget (
 invocation_id TEXT PRIMARY KEY REFERENCES tool_invocation(invocation_id),
 run_id TEXT NOT NULL REFERENCES run(run_id),
 ui_ref TEXT NOT NULL UNIQUE,
 document_json TEXT NOT NULL CHECK(json_valid(document_json)),
 message_id TEXT REFERENCES conversation_intent(message_id) DEFERRABLE INITIALLY DEFERRED,
 created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX ui_widget_message ON ui_widget(message_id,created_at,invocation_id);
CREATE TRIGGER ui_widget_immutable BEFORE UPDATE ON ui_widget
WHEN NEW.invocation_id!=OLD.invocation_id OR NEW.run_id!=OLD.run_id OR NEW.ui_ref!=OLD.ui_ref OR NEW.document_json!=OLD.document_json OR NEW.created_at!=OLD.created_at OR OLD.message_id IS NOT NULL
BEGIN SELECT RAISE(ABORT,'UI artifact is immutable'); END;
