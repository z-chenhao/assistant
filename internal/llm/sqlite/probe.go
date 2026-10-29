package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/z-chenhao/collie/internal/llm"
)

func (s *Store) RecordProbe(ctx context.Context, p llm.Profile, r llm.ProbeRecord) error {
	if err := p.ValidateActivation(); err != nil {
		return err
	}
	if r.Result.ValidateCapabilities(p.Capabilities) != nil || r.ProfileVersion != p.Version || r.BridgeIdentity == "" {
		return llm.ErrUnverified
	}
	encoded, err := llm.CanonicalJSON(r.Result)
	if err != nil {
		return err
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		var version int64
		if err := tx.QueryRowContext(ctx, `SELECT version FROM model_profile WHERE profile_id=?`, p.ProfileID).Scan(&version); err != nil {
			return err
		}
		if version != p.Version {
			return llm.ErrConflict
		}
		if p.AuthConnectionID != "" {
			var authVersion int64
			if err := tx.QueryRowContext(ctx, `SELECT version FROM auth_connection WHERE connection_id=? AND status='connected' AND (expires_at IS NULL OR expires_at>?)`, p.AuthConnectionID, now().UnixMicro()).Scan(&authVersion); err != nil {
				return err
			}
			if authVersion != r.AuthVersion {
				return llm.ErrConflict
			}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO model_probe(profile_id,profile_version,auth_version,bridge_identity,result_json,verified_at) VALUES(?,?,?,?,?,?) ON CONFLICT(profile_id) DO UPDATE SET profile_version=excluded.profile_version,auth_version=excluded.auth_version,bridge_identity=excluded.bridge_identity,result_json=excluded.result_json,verified_at=excluded.verified_at`, p.ProfileID, p.Version, r.AuthVersion, r.BridgeIdentity, string(encoded), now().UnixMicro())
		if err != nil {
			return err
		}
		return enqueue(ctx, tx, "model_profile.probed", p.ProfileID, p.Version)
	})
}

func (s *Store) Probe(ctx context.Context, id string) (llm.ProbeRecord, error) {
	var r llm.ProbeRecord
	var encoded string
	err := s.db.QueryRowContext(ctx, `SELECT profile_version,auth_version,bridge_identity,result_json FROM model_probe WHERE profile_id=?`, id).Scan(&r.ProfileVersion, &r.AuthVersion, &r.BridgeIdentity, &encoded)
	if err == sql.ErrNoRows {
		return r, llm.ErrNotFound
	}
	if err != nil || json.Unmarshal([]byte(encoded), &r.Result) != nil {
		return r, llm.ErrStorage
	}
	return r, nil
}

// A failed re-test revokes only proof for the tested version. It cannot erase
// newer evidence or mutate a profile edited while the Probe was running.
func (s *Store) InvalidateProbe(ctx context.Context, id string, version int64, status string) error {
	if status != "invalid" && status != "unavailable" && status != "reauth_required" {
		return llm.ErrStorage
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, `DELETE FROM model_probe WHERE profile_id=? AND profile_version=? AND EXISTS(SELECT 1 FROM model_profile p WHERE p.profile_id=? AND p.version=?)`, id, version, id, version)
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if _, err := tx.ExecContext(ctx, `UPDATE model_profile SET status=?,verified_at=NULL,version=version+1,updated_at=max(updated_at,?) WHERE profile_id=? AND version=?`, status, now().UnixMicro(), id, version); err != nil {
			return err
		}
		r, err = tx.ExecContext(ctx, `UPDATE agent_config SET setup_status='degraded',config_version=config_version+1,updated_at=max(updated_at,?) WHERE active_model_profile_id=?`, now().UnixMicro(), id)
		if err != nil {
			return err
		}
		n, err = r.RowsAffected()
		if err != nil {
			return err
		}
		if n != 0 {
			var agentID string
			var configVersion int64
			if err := tx.QueryRowContext(ctx, `SELECT agent_id,config_version FROM agent_config WHERE singleton=1`).Scan(&agentID, &configVersion); err != nil {
				return err
			}
			if err := enqueue(ctx, tx, "agent.model_degraded", agentID, configVersion); err != nil {
				return err
			}
		}
		return enqueue(ctx, tx, "model_profile.probe_invalidated", id, version+1)
	})
}

// Select commits the verified profile, stable AuthConnection reference, Agent
// config CAS, and Outbox together. An interrupted/failed transaction activates
// nothing. SecretStore material has already been checked by Control.
func (s *Store) Select(ctx context.Context, id string, profileVersion, configVersion int64, identity string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var encoded string
		var authVersion int64
		if err := tx.QueryRowContext(ctx, `SELECT result_json,auth_version FROM model_probe WHERE profile_id=? AND profile_version=? AND bridge_identity=?`, id, profileVersion, identity).Scan(&encoded, &authVersion); err != nil {
			return llm.ErrConflict
		}
		var result llm.ProbeResult
		if json.Unmarshal([]byte(encoded), &result) != nil || result.Validate() != nil {
			return llm.ErrConflict
		}
		p, err := scanProfile(tx.QueryRowContext(ctx, `SELECT `+profileColumns+` FROM model_profile WHERE profile_id=?`, id))
		if err != nil {
			return err
		}
		if p.Version != profileVersion {
			return llm.ErrConflict
		}
		if err := p.ValidateActivation(); err != nil {
			return err
		}
		if result.ValidateCapabilities(p.Capabilities) != nil {
			return llm.ErrConflict
		}
		if p.AuthConnectionID != "" {
			var version int64
			if err := tx.QueryRowContext(ctx, `SELECT version FROM auth_connection WHERE connection_id=? AND status='connected' AND (expires_at IS NULL OR expires_at>?)`, p.AuthConnectionID, now().UnixMicro()).Scan(&version); err != nil {
				return llm.ErrConflict
			}
			if version != authVersion {
				return llm.ErrConflict
			}
		}
		// Only observed capabilities survive activation. Audio, compatibility,
		// reasoning and sampling claims still have no behavioral proof.
		normalized := p.ProfileInput
		normalized.Compatibility = llm.Compatibility{ToolResultFormat: "openai"}
		normalized.Capabilities = llm.Capabilities{TextInput: true, ToolCalls: true, Streaming: true, ImageInput: p.Capabilities.ImageInput && result.ImageInput, ReasoningEfforts: []string{}}
		normalized.GenerationDefaults = llm.GenerationDefaults{}
		g, compat, caps, err := encode(normalized)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE model_profile SET status='ready',verified_at=(SELECT verified_at FROM model_probe WHERE profile_id=?),generation_defaults_json=?,compatibility_json=?,capabilities_json=?,updated_at=max(updated_at,?),version=version+1 WHERE profile_id=? AND version=?`, id, g, compat, caps, now().UnixMicro(), id, profileVersion)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE model_probe SET profile_version=profile_version+1 WHERE profile_id=?`, id); err != nil {
			return err
		}
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
	})
}
