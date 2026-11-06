-- Mutable presentation state is separate from immutable delivered artifacts.
CREATE TABLE ui_view_state (
 component_ref TEXT PRIMARY KEY,
 state_json TEXT NOT NULL CHECK(json_valid(state_json)),
 model_context_json TEXT NOT NULL CHECK(json_valid(model_context_json)),
 version INTEGER NOT NULL CHECK(version>0),
 updated_at INTEGER NOT NULL
) STRICT;
CREATE TABLE main_ui_context (
 message_id TEXT PRIMARY KEY REFERENCES main_input(message_id),
 context_json TEXT NOT NULL CHECK(json_valid(context_json))
) STRICT;
CREATE TABLE ui_action_request (
 message_id TEXT PRIMARY KEY REFERENCES main_input(message_id),
 component_ref TEXT NOT NULL,
 source_id TEXT NOT NULL UNIQUE,
 definition_hash TEXT NOT NULL CHECK(length(definition_hash)=64),
 tool_name TEXT NOT NULL,
 arguments_json TEXT NOT NULL CHECK(json_valid(arguments_json)),
 step_floor INTEGER,
 step_ceiling INTEGER,
 reply_message_id TEXT REFERENCES conversation_intent(message_id) DEFERRABLE INITIALLY DEFERRED
) STRICT;
CREATE TRIGGER ui_view_state_version BEFORE UPDATE ON ui_view_state
WHEN NEW.component_ref!=OLD.component_ref OR NEW.version!=OLD.version+1
BEGIN SELECT RAISE(ABORT,'invalid UI state version'); END;
CREATE TRIGGER main_ui_context_immutable BEFORE UPDATE ON main_ui_context BEGIN SELECT RAISE(ABORT,'immutable UI input snapshot'); END;
CREATE TRIGGER main_ui_context_no_delete BEFORE DELETE ON main_ui_context BEGIN SELECT RAISE(ABORT,'UI input audit retained'); END;
CREATE TRIGGER ui_action_request_immutable BEFORE UPDATE ON ui_action_request
WHEN NEW.message_id!=OLD.message_id OR NEW.component_ref!=OLD.component_ref OR NEW.source_id!=OLD.source_id OR NEW.definition_hash!=OLD.definition_hash OR NEW.tool_name!=OLD.tool_name OR NEW.arguments_json!=OLD.arguments_json OR (OLD.step_floor IS NOT NULL AND NEW.step_floor IS NOT OLD.step_floor) OR (OLD.step_ceiling IS NOT NULL AND NEW.step_ceiling IS NOT OLD.step_ceiling) OR (OLD.reply_message_id IS NOT NULL AND NEW.reply_message_id IS NOT OLD.reply_message_id)
BEGIN SELECT RAISE(ABORT,'immutable UI action request'); END;
CREATE TRIGGER ui_action_request_no_delete BEFORE DELETE ON ui_action_request BEGIN SELECT RAISE(ABORT,'UI action audit retained'); END;
