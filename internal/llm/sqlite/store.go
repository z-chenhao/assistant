// Package sqlite persists model metadata in the Host-owned state.db. Errors at
// this boundary are deliberately value-free; SQLite errors can echo inputs.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/z-chenhao/collie/internal/event"
	eventsqlite "github.com/z-chenhao/collie/internal/event/sqlite"
	"github.com/z-chenhao/collie/internal/llm"
)

type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

func now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }
func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func micros(v *time.Time) any {
	if v == nil {
		return nil
	}
	return v.UnixMicro()
}
func timestamp(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := time.UnixMicro(v.Int64).UTC()
	return &t
}

func (s *Store) write(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return llm.ErrStorage
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		if errors.Is(err, llm.ErrConflict) {
			return llm.ErrConflict
		}
		return llm.ErrStorage
	}
	if err := tx.Commit(); err != nil {
		return llm.ErrStorage
	}
	return nil
}

func enqueue(ctx context.Context, tx *sql.Tx, kind, id string, version int64) error {
	e, err := event.New("/collie/models", "ai.collie."+kind+".v1", id, map[string]any{"id": id, "version": version})
	if err != nil {
		return err
	}
	return eventsqlite.Enqueue(ctx, tx, e)
}

func (s *Store) AddProfile(ctx context.Context, input llm.ProfileInput) (llm.Profile, error) {
	if err := input.Validate(); err != nil {
		return llm.Profile{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return llm.Profile{}, llm.ErrStorage
	}
	t := now()
	p := llm.Profile{ProfileInput: input, ProfileID: id.String(), Status: "unverified", CreatedAt: t, UpdatedAt: t, Version: 1}
	g, c, caps, err := encode(input)
	if err != nil {
		return llm.Profile{}, err
	}
	err = s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO model_profile (`+profileColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.ProfileID, input.Name, input.ProviderKind, nullable(input.Endpoint), input.WireAPI, input.ModelID, nullable(input.AuthConnectionID), nullable(input.ReasoningEffort), input.ContextWindow, input.MaxOutputTokens,
			g, c, caps, p.Status, nil, t.UnixMicro(), t.UnixMicro(), p.Version)
		if err != nil {
			return err
		}
		if err := enqueue(ctx, tx, "model_profile.added", p.ProfileID, 1); err != nil {
			return err
		}
		p, err = scanProfile(tx.QueryRowContext(ctx, `SELECT `+profileColumns+` FROM model_profile WHERE profile_id=?`, p.ProfileID))
		return err
	})
	if err != nil {
		return llm.Profile{}, err
	}
	return p, nil
}

// UpdateProfile changes metadata using CAS and invalidates prior verification.
// There is deliberately no ready/verified/activate setter on this control path.
func (s *Store) UpdateProfile(ctx context.Context, id string, expected int64, input llm.ProfileInput) (llm.Profile, error) {
	if err := input.Validate(); err != nil {
		return llm.Profile{}, err
	}
	if expected < 1 {
		return llm.Profile{}, llm.ErrConflict
	}
	g, c, caps, err := encode(input)
	if err != nil {
		return llm.Profile{}, err
	}
	var updated llm.Profile
	err = s.write(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, `UPDATE model_profile SET name=?,provider_kind=?,endpoint=?,wire_api=?,model_id=?,auth_connection_id=?,reasoning_effort=?,context_window=?,max_output_tokens=?,generation_defaults_json=?,compatibility_json=?,capabilities_json=?,status='unverified',verified_at=NULL,updated_at=max(updated_at,?),version=version+1 WHERE profile_id=? AND version=?`,
			input.Name, input.ProviderKind, nullable(input.Endpoint), input.WireAPI, input.ModelID, nullable(input.AuthConnectionID), nullable(input.ReasoningEffort), input.ContextWindow, input.MaxOutputTokens, g, c, caps, now().UnixMicro(), id, expected)
		if err != nil {
			return err
		}
		count, err := r.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return llm.ErrConflict
		}
		if err := enqueue(ctx, tx, "model_profile.updated", id, expected+1); err != nil {
			return err
		}
		updated, err = scanProfile(tx.QueryRowContext(ctx, `SELECT `+profileColumns+` FROM model_profile WHERE profile_id=?`, id))
		return err
	})
	if err != nil {
		return llm.Profile{}, err
	}
	return updated, nil
}

func encode(p llm.ProfileInput) (string, string, string, error) {
	g, err := llm.CanonicalJSON(p.GenerationDefaults)
	if err != nil {
		return "", "", "", err
	}
	c, err := llm.CanonicalJSON(p.Compatibility)
	if err != nil {
		return "", "", "", err
	}
	caps := p.Capabilities
	if caps.ReasoningEfforts == nil {
		caps.ReasoningEfforts = []string{}
	}
	b, err := llm.CanonicalJSON(caps)
	return string(g), string(c), string(b), err
}

const profileColumns = `profile_id,name,provider_kind,endpoint,wire_api,model_id,auth_connection_id,reasoning_effort,context_window,max_output_tokens,generation_defaults_json,compatibility_json,capabilities_json,status,verified_at,created_at,updated_at,version`

type scanner interface{ Scan(...any) error }

func scanProfile(row scanner) (llm.Profile, error) {
	var p llm.Profile
	var endpoint, auth, effort sql.NullString
	var verified sql.NullInt64
	var created, updated int64
	var g, c, caps string
	err := row.Scan(&p.ProfileID, &p.Name, &p.ProviderKind, &endpoint, &p.WireAPI, &p.ModelID, &auth, &effort, &p.ContextWindow, &p.MaxOutputTokens, &g, &c, &caps, &p.Status, &verified, &created, &updated, &p.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return p, llm.ErrNotFound
	}
	if err != nil {
		return p, llm.ErrStorage
	}
	p.Endpoint, p.AuthConnectionID, p.ReasoningEffort = endpoint.String, auth.String, effort.String
	p.CreatedAt, p.UpdatedAt, p.VerifiedAt = time.UnixMicro(created).UTC(), time.UnixMicro(updated).UTC(), timestamp(verified)
	if json.Unmarshal([]byte(g), &p.GenerationDefaults) != nil || json.Unmarshal([]byte(c), &p.Compatibility) != nil || json.Unmarshal([]byte(caps), &p.Capabilities) != nil {
		return llm.Profile{}, llm.ErrStorage
	}
	if p.Validate() != nil {
		return llm.Profile{}, llm.ErrStorage
	}
	return p, nil
}

func (s *Store) Profile(ctx context.Context, id string) (llm.Profile, error) {
	return scanProfile(s.db.QueryRowContext(ctx, `SELECT `+profileColumns+` FROM model_profile WHERE profile_id=?`, id))
}

func (s *Store) ListProfiles(ctx context.Context) ([]llm.Profile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+profileColumns+` FROM model_profile ORDER BY profile_id`)
	if err != nil {
		return nil, llm.ErrStorage
	}
	defer rows.Close()
	profiles := []llm.Profile{}
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	if rows.Err() != nil {
		return nil, llm.ErrStorage
	}
	return profiles, nil
}

func (s *Store) InsertAuth(ctx context.Context, a llm.AuthConnection) error {
	if a.AuthInput.Validate() != nil || a.Status != "connected" || a.Version != 1 || a.CredentialRef == "" {
		return llm.ErrStorage
	}
	id, err := uuid.Parse(a.ConnectionID)
	if err != nil || id.Version() != 7 {
		return llm.ErrStorage
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO auth_connection (connection_id,provider,auth_kind,credential_ref,account_label,workspace_label,status,expires_at,last_refreshed_at,created_at,updated_at,version) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, a.ConnectionID, a.Provider, a.AuthKind, a.CredentialRef, nullable(a.AccountLabel), nullable(a.WorkspaceLabel), a.Status, micros(a.ExpiresAt), micros(a.LastRefreshedAt), a.CreatedAt.UnixMicro(), a.UpdatedAt.UnixMicro(), a.Version)
		if err != nil {
			return err
		}
		return enqueue(ctx, tx, "auth_connection.added", a.ConnectionID, 1)
	})
}

func (s *Store) Auth(ctx context.Context, id string) (llm.AuthConnection, error) {
	var a llm.AuthConnection
	var account, workspace sql.NullString
	var expires, refreshed sql.NullInt64
	var created, updated int64
	err := s.db.QueryRowContext(ctx, `SELECT connection_id,provider,auth_kind,credential_ref,account_label,workspace_label,status,expires_at,last_refreshed_at,created_at,updated_at,version FROM auth_connection WHERE connection_id=?`, id).Scan(&a.ConnectionID, &a.Provider, &a.AuthKind, &a.CredentialRef, &account, &workspace, &a.Status, &expires, &refreshed, &created, &updated, &a.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return a, llm.ErrNotFound
	}
	if err != nil {
		return a, llm.ErrStorage
	}
	a.AccountLabel, a.WorkspaceLabel = account.String, workspace.String
	a.ExpiresAt, a.LastRefreshedAt = timestamp(expires), timestamp(refreshed)
	a.CreatedAt, a.UpdatedAt = time.UnixMicro(created).UTC(), time.UnixMicro(updated).UTC()
	return a, nil
}

// UpdateAuthState changes persisted authentication metadata only. It does not
// refresh credentials, perform logout, or change stable account identity.
func (s *Store) UpdateAuthState(ctx context.Context, id string, expected int64, status string, expires, refreshed *time.Time) error {
	switch status {
	case "connected", "refreshing", "reauth_required", "logged_out":
	default:
		return errors.New("invalid authentication status")
	}
	if expected < 1 {
		return llm.ErrConflict
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, `UPDATE auth_connection SET status=?,expires_at=?,last_refreshed_at=?,updated_at=max(updated_at,?),version=version+1 WHERE connection_id=? AND version=?`, status, micros(expires), micros(refreshed), now().UnixMicro(), id, expected)
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
		return enqueue(ctx, tx, "auth_connection.updated", id, expected+1)
	})
}

// FreezeProfile must use the future Run owner's transaction so a snapshot and
// Run can commit together. No CLI exposes this path. Unverified profiles fail.
func (s *Store) FreezeProfile(ctx context.Context, tx *sql.Tx, id string, expected int64) (llm.Snapshot, error) {
	p, err := scanProfile(tx.QueryRowContext(ctx, `SELECT `+profileColumns+` FROM model_profile WHERE profile_id=?`, id))
	if err != nil {
		return llm.Snapshot{}, err
	}
	if p.Version != expected {
		return llm.Snapshot{}, llm.ErrConflict
	}
	if p.Status != "ready" || p.VerifiedAt == nil {
		return llm.Snapshot{}, llm.ErrUnverified
	}
	if p.AuthConnectionID != "" {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM auth_connection WHERE connection_id=?`, p.AuthConnectionID).Scan(&status); err != nil {
			return llm.Snapshot{}, llm.ErrStorage
		}
		if status != "connected" {
			return llm.Snapshot{}, llm.ErrUnverified
		}
	}
	snapshotID, err := uuid.NewV7()
	if err != nil {
		return llm.Snapshot{}, llm.ErrStorage
	}
	hash, err := llm.EndpointHash(p.Endpoint)
	if err != nil {
		return llm.Snapshot{}, err
	}
	v := llm.Snapshot{SnapshotID: snapshotID.String(), ProfileID: id, ProfileVersion: p.Version, ProviderKind: p.ProviderKind, EndpointHash: hash, WireAPI: p.WireAPI, ModelID: p.ModelID, AuthConnectionID: p.AuthConnectionID, ReasoningEffort: p.ReasoningEffort, ContextWindow: p.ContextWindow, MaxOutputTokens: p.MaxOutputTokens, GenerationDefaults: p.GenerationDefaults, Compatibility: p.Compatibility, Capabilities: p.Capabilities, CreatedAt: now()}
	g, c, caps, err := encode(p.ProfileInput)
	if err != nil {
		return llm.Snapshot{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO model_snapshot (`+snapshotColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, v.SnapshotID, v.ProfileID, v.ProfileVersion, v.ProviderKind, nullable(v.EndpointHash), v.WireAPI, v.ModelID, nullable(v.AuthConnectionID), nullable(v.ReasoningEffort), v.ContextWindow, v.MaxOutputTokens, g, c, caps, v.CreatedAt.UnixMicro())
	if err != nil {
		return llm.Snapshot{}, llm.ErrStorage
	}
	if err := enqueue(ctx, tx, "model_snapshot.created", v.SnapshotID, 1); err != nil {
		return llm.Snapshot{}, llm.ErrStorage
	}
	return v, nil
}

const snapshotColumns = `snapshot_id,profile_id,profile_version,provider_kind,endpoint_hash,wire_api,model_id,auth_connection_id,reasoning_effort,context_window,max_output_tokens,generation_defaults_json,compatibility_json,capabilities_json,created_at`

func (s *Store) Snapshot(ctx context.Context, id string) (llm.Snapshot, error) {
	var v llm.Snapshot
	var hash, auth, effort sql.NullString
	var g, c, caps string
	var created int64
	err := s.db.QueryRowContext(ctx, `SELECT `+snapshotColumns+` FROM model_snapshot WHERE snapshot_id=?`, id).Scan(&v.SnapshotID, &v.ProfileID, &v.ProfileVersion, &v.ProviderKind, &hash, &v.WireAPI, &v.ModelID, &auth, &effort, &v.ContextWindow, &v.MaxOutputTokens, &g, &c, &caps, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return v, llm.ErrNotFound
	}
	if err != nil {
		return v, llm.ErrStorage
	}
	v.EndpointHash, v.AuthConnectionID, v.ReasoningEffort = hash.String, auth.String, effort.String
	v.CreatedAt = time.UnixMicro(created).UTC()
	if json.Unmarshal([]byte(g), &v.GenerationDefaults) != nil || json.Unmarshal([]byte(c), &v.Compatibility) != nil || json.Unmarshal([]byte(caps), &v.Capabilities) != nil {
		return llm.Snapshot{}, llm.ErrStorage
	}
	return v, nil
}
