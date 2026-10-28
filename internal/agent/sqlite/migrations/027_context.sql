-- Version zero remains the frozen pre-working-state representation. A Run's
-- context format is immutable; new code never upgrades an accepted checkpoint.
CREATE TRIGGER IF NOT EXISTS run_context_initial BEFORE INSERT ON run WHEN
 coalesce(json_extract(NEW.checkpoint_json,'$.context_version'),0) NOT IN (0,2)
 OR json_type(NEW.checkpoint_json,'$.working_state') NOT IN ('null')
 BEGIN SELECT RAISE(ABORT,'invalid initial context'); END;

CREATE TRIGGER IF NOT EXISTS run_context_version BEFORE UPDATE ON run WHEN
 coalesce(json_extract(NEW.checkpoint_json,'$.context_version'),0) != coalesce(json_extract(OLD.checkpoint_json,'$.context_version'),0)
 BEGIN SELECT RAISE(ABORT,'immutable context version'); END;

CREATE TRIGGER IF NOT EXISTS run_working_state BEFORE UPDATE ON run WHEN
 json_type(NEW.checkpoint_json,'$.working_state') IS NOT NULL
 AND json_type(NEW.checkpoint_json,'$.working_state') != 'null'
 AND (
  coalesce(json_extract(NEW.checkpoint_json,'$.context_version'),0) != 2
  OR json_type(NEW.checkpoint_json,'$.working_state') != 'object'
  OR coalesce(json_type(NEW.checkpoint_json,'$.working_state.through'),'') != 'integer'
  OR json_extract(NEW.checkpoint_json,'$.working_state.through') <= 0
  OR json_extract(NEW.checkpoint_json,'$.working_state.through') > json_array_length(NEW.checkpoint_json,'$.messages')
  OR json_extract(NEW.checkpoint_json,'$.working_state.through') < coalesce(json_extract(OLD.checkpoint_json,'$.working_state.through'),0)
  OR length(coalesce(json_extract(NEW.checkpoint_json,'$.working_state.text'),'')) = 0
  OR coalesce(json_type(NEW.checkpoint_json,'$.working_state.source_refs'),'') != 'array'
  OR NOT EXISTS(SELECT 1 FROM artifact a WHERE a.artifact_ref=json_extract(NEW.checkpoint_json,'$.working_state.archive_ref') AND a.run_id=NEW.run_id AND a.visibility='run')
 )
 BEGIN SELECT RAISE(ABORT,'invalid working state'); END;
