package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"github.com/z-chenhao/erii/internal/llm"
)

func (s *Store) DeviceID(ctx context.Context) (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", llm.ErrStorage
	}
	if _, err = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO installation_auth VALUES(1,?)`, id.String()); err != nil {
		return "", llm.ErrStorage
	}
	var value string
	if s.db.QueryRowContext(ctx, `SELECT device_id FROM installation_auth WHERE singleton=1`).Scan(&value) != nil {
		return "", llm.ErrStorage
	}
	return value, nil
}
func (s *Store) Accounts(ctx context.Context) ([]llm.AuthConnection, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT connection_id FROM auth_connection WHERE provider='openai' AND auth_kind='chatgpt_oauth' ORDER BY connection_id`)
	if err != nil {
		return nil, llm.ErrStorage
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			return nil, llm.ErrStorage
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, llm.ErrStorage
	}
	accounts := []llm.AuthConnection{}
	for _, id := range ids {
		a, err := s.Auth(ctx, id)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, nil
}
func (s *Store) Registration(ctx context.Context, id string) (llm.AccountRegistration, error) {
	var r llm.AccountRegistration
	err := s.db.QueryRowContext(ctx, `SELECT client_id,subject_hash,auth_epoch FROM account_registration WHERE connection_id=?`, id).Scan(&r.ClientID, &r.SubjectHash, &r.AuthEpoch)
	if errors.Is(err, sql.ErrNoRows) {
		return r, llm.AuthRequired
	}
	if err != nil {
		return r, llm.ErrStorage
	}
	return r, nil
}
func (s *Store) InsertAccount(ctx context.Context, a llm.AuthConnection, r llm.AccountRegistration) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO auth_connection(connection_id,provider,auth_kind,credential_ref,account_label,workspace_label,status,expires_at,created_at,updated_at,version) VALUES(?,'openai','chatgpt_oauth',?,?,?,'connected',?,?,?,1)`, a.ConnectionID, a.CredentialRef, a.AccountLabel, a.WorkspaceLabel, micros(a.ExpiresAt), a.CreatedAt.UnixMicro(), a.UpdatedAt.UnixMicro())
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO account_registration(connection_id,client_id,subject_hash) VALUES(?,?,?)`, a.ConnectionID, r.ClientID, r.SubjectHash); err != nil {
			return err
		}
		return enqueue(ctx, tx, "auth_connection.added", a.ConnectionID, 1)
	})
}

// AccountState commits auth CAS, readiness/proof changes and events together.
// Refresh of the same identity preserves capability proof; auth loss revokes it.
func (s *Store) AccountState(ctx context.Context, a llm.AuthConnection, status string, credential *llm.OAuthCredential) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var expires any
		refreshed := micros(a.LastRefreshedAt)
		if credential != nil {
			expires = credential.ExpiresAt.UnixMicro()
			var registration llm.AccountRegistration
			if tx.QueryRowContext(ctx, `SELECT client_id,subject_hash FROM account_registration WHERE connection_id=?`, a.ConnectionID).Scan(&registration.ClientID, &registration.SubjectHash) != nil || credential.ClientID != registration.ClientID || credential.SubjectHash != registration.SubjectHash {
				return llm.AuthRequired
			}
			refreshed = now().UnixMicro()
		} else {
			expires = micros(a.ExpiresAt)
		}
		r, err := tx.ExecContext(ctx, `UPDATE auth_connection SET status=?,expires_at=?,last_refreshed_at=?,updated_at=max(updated_at,?),version=version+1 WHERE connection_id=? AND version=?`, status, expires, refreshed, now().UnixMicro(), a.ConnectionID, a.Version)
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
		if status == "connected" && a.Status == "refreshing" {
			if _, err = tx.ExecContext(ctx, `UPDATE model_probe SET auth_version=? WHERE profile_id IN (SELECT profile_id FROM model_profile WHERE auth_connection_id=?) AND auth_version=?`, a.Version+1, a.ConnectionID, a.Version-1); err != nil {
				return err
			}
		}
		if status == "reauth_required" || status == "logged_out" {
			if _, err = tx.ExecContext(ctx, `UPDATE account_registration SET auth_epoch=auth_epoch+1 WHERE connection_id=?`, a.ConnectionID); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `DELETE FROM model_probe WHERE profile_id IN(SELECT profile_id FROM model_profile WHERE auth_connection_id=?)`, a.ConnectionID); err != nil {
				return err
			}
			rows, err := tx.QueryContext(ctx, `SELECT profile_id,version FROM model_profile WHERE auth_connection_id=?`, a.ConnectionID)
			if err != nil {
				return err
			}
			type item struct {
				id string
				v  int64
			}
			var items []item
			for rows.Next() {
				var i item
				if err = rows.Scan(&i.id, &i.v); err != nil {
					rows.Close()
					return err
				}
				items = append(items, i)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
			for _, i := range items {
				if _, err = tx.ExecContext(ctx, `UPDATE model_profile SET status='reauth_required',verified_at=NULL,updated_at=max(updated_at,?),version=version+1 WHERE profile_id=?`, now().UnixMicro(), i.id); err != nil {
					return err
				}
				if err = enqueue(ctx, tx, "model_profile.auth_lost", i.id, i.v+1); err != nil {
					return err
				}
			}
			r, err = tx.ExecContext(ctx, `UPDATE agent_config SET setup_status='degraded',config_version=config_version+1,updated_at=max(updated_at,?) WHERE setup_status='ready' AND active_model_profile_id IN(SELECT profile_id FROM model_profile WHERE auth_connection_id=?)`, now().UnixMicro(), a.ConnectionID)
			if err != nil {
				return err
			}
			n, err = r.RowsAffected()
			if err != nil {
				return err
			}
			if n > 0 {
				var id string
				var v int64
				if tx.QueryRowContext(ctx, `SELECT agent_id,config_version FROM agent_config WHERE singleton=1`).Scan(&id, &v) != nil {
					return llm.ErrStorage
				}
				if err = enqueue(ctx, tx, "agent.model_degraded", id, v); err != nil {
					return err
				}
			}
		}
		return enqueue(ctx, tx, "auth_connection.updated", a.ConnectionID, a.Version+1)
	})
}
