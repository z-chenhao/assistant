DROP TRIGGER agent_config_select_ready;
CREATE TRIGGER agent_config_select_ready BEFORE UPDATE OF active_model_profile_id,setup_status ON agent_config
WHEN NEW.setup_status = 'ready' AND NOT EXISTS (
    SELECT 1 FROM model_profile p
    WHERE p.profile_id=NEW.active_model_profile_id AND p.status='ready' AND p.verified_at IS NOT NULL
      AND ((p.provider_kind='openai_account' AND EXISTS (
          SELECT 1 FROM auth_connection a JOIN account_registration r ON r.connection_id=a.connection_id
          WHERE a.connection_id=p.auth_connection_id AND a.auth_kind='chatgpt_oauth' AND a.status='connected'))
        OR (p.provider_kind!='openai_account' AND EXISTS (
          SELECT 1 FROM model_probe v WHERE v.profile_id=p.profile_id AND v.profile_version=p.version
            AND (p.auth_connection_id IS NULL OR EXISTS (
              SELECT 1 FROM auth_connection a WHERE a.connection_id=p.auth_connection_id
                AND a.status='connected' AND a.version=v.auth_version))))))
BEGIN SELECT RAISE(ABORT, 'active profile requires current authorization'); END;
