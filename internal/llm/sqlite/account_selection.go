package sqlite

import (
	"context"
	"database/sql"

	"github.com/z-chenhao/assistant/internal/llm"
)

// SelectAccount uses authenticated catalog metadata. It never manufactures a
// behavioral Probe. Profile, auth and Agent versions are fenced in one write.
func (s *Store) SelectAccount(ctx context.Context, selection llm.AccountSelection) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		p, err := scanProfile(tx.QueryRowContext(ctx, `SELECT `+profileColumns+` FROM model_profile WHERE profile_id=?`, selection.ProfileID))
		if err != nil {
			return err
		}
		if p.Version != selection.ProfileVersion || p.ProviderKind != "openai_account" || p.ModelID != selection.Model.ModelID {
			return llm.ErrConflict
		}
		var authVersion int64
		if err := tx.QueryRowContext(ctx, `SELECT version FROM auth_connection WHERE connection_id=? AND auth_kind='chatgpt_oauth' AND status='connected' AND (expires_at IS NULL OR expires_at>?)`, p.AuthConnectionID, now().UnixMicro()).Scan(&authVersion); err != nil || authVersion != selection.AuthVersion {
			return llm.ErrConflict
		}
		p.ContextWindow = selection.Model.ContextWindow
		p.MaxOutputTokens = selection.Model.MaxOutputTokens
		p.Capabilities = llm.Capabilities{TextInput: true, ImageInput: selection.Model.ImageInput, ToolCalls: true, Streaming: true}
		p.Compatibility = llm.Compatibility{ToolResultFormat: "openai"}
		p.GenerationDefaults = llm.GenerationDefaults{}
		if err := p.ValidateActivation(); err != nil {
			return err
		}
		g, compat, caps, err := encode(p.ProfileInput)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE model_profile SET status='ready',verified_at=?,context_window=?,max_output_tokens=?,generation_defaults_json=?,compatibility_json=?,capabilities_json=?,updated_at=max(updated_at,?),version=version+1 WHERE profile_id=? AND version=?`, now().UnixMicro(), p.ContextWindow, p.MaxOutputTokens, g, compat, caps, now().UnixMicro(), p.ProfileID, p.Version)
		if err != nil {
			return err
		}
		return selectAgentModel(ctx, tx, p.ProfileID, p.Version, selection.ConfigVersion)
	})
}

func selectAgentModel(ctx context.Context, tx *sql.Tx, id string, profileVersion, configVersion int64) error {
	r, err := tx.ExecContext(ctx, `UPDATE agent_config SET active_model_profile_id=?,setup_status='ready',config_version=config_version+1,updated_at=max(updated_at,?) WHERE singleton=1 AND config_version=?`, id, now().UnixMicro(), configVersion)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return llm.ErrConflict
	}
	if err := enqueue(ctx, tx, "model_profile.activated", id, profileVersion+1); err != nil {
		return err
	}
	var agentID string
	if err := tx.QueryRowContext(ctx, `SELECT agent_id FROM agent_config WHERE singleton=1`).Scan(&agentID); err != nil {
		return err
	}
	return enqueue(ctx, tx, "agent.model_selected", agentID, configVersion+1)
}
