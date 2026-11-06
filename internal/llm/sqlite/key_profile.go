package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/z-chenhao/assistant/internal/llm"
)

// The secret is persisted by RegisterAuth; its metadata and the first profile
// commit atomically. A rejected transaction removes the newly written file.
func (s *Store) AddKeyProfile(ctx context.Context, input llm.ProfileInput, secrets llm.SecretStore, key []byte) (llm.Profile, error) {
	if input.ProviderKind != "openai_compatible" || input.AuthConnectionID != "" || input.Validate() != nil {
		return llm.Profile{}, llm.ModelIncompatible
	}
	if secrets == nil || len(key) == 0 || len(key) > 16384 {
		return llm.Profile{}, llm.ErrSecretStore
	}
	writer := &keyProfileWriter{store: s, input: input}
	_, err := llm.RegisterAuth(ctx, writer, secrets, llm.AuthInput{Provider: "openai_compatible", AuthKind: "api_key", AccountLabel: input.Name}, key)
	if err != nil {
		return llm.Profile{}, err
	}
	return writer.profile, nil
}

type keyProfileWriter struct {
	store   *Store
	input   llm.ProfileInput
	profile llm.Profile
}

func (w *keyProfileWriter) InsertAuth(ctx context.Context, a llm.AuthConnection) error {
	input := w.input
	input.AuthConnectionID = a.ConnectionID
	if input.Validate() != nil {
		return llm.ModelIncompatible
	}
	id, err := uuid.NewV7()
	if err != nil {
		return llm.ErrStorage
	}
	t := now()
	p := llm.Profile{ProfileInput: input, ProfileID: id.String(), Status: "unverified", CreatedAt: t, UpdatedAt: t, Version: 1}
	g, c, caps, err := encode(input)
	if err != nil {
		return err
	}
	err = w.store.write(ctx, func(tx *sql.Tx) error {
		if err := insertAuth(ctx, tx, a); err != nil {
			return err
		}
		return insertProfile(ctx, tx, &p, g, c, caps)
	})
	if err == nil {
		w.profile = p
	}
	return err
}

// Disconnect keeps historical metadata and frozen work, revokes live use of
// this key and removes its file only after the versioned database commit.
func (s *Store) DisconnectKeyProfile(ctx context.Context, id string, version, authVersion int64, secrets llm.SecretStore) error {
	p, err := s.Profile(ctx, id)
	if err != nil || p.Version != version || p.AuthConnectionID == "" {
		return llm.ErrConflict
	}
	a, err := s.Auth(ctx, p.AuthConnectionID)
	if err != nil || a.AuthKind != "api_key" || a.Version != authVersion {
		return llm.ErrConflict
	}
	if secrets == nil {
		return llm.ErrSecretStore
	}
	err = s.write(ctx, func(tx *sql.Tx) error {
		var actual int64
		if err := tx.QueryRowContext(ctx, `SELECT version FROM model_profile WHERE profile_id=?`, id).Scan(&actual); err != nil || actual != version {
			return llm.ErrConflict
		}
		return accountState(ctx, tx, a, "logged_out", nil)
	})
	if err != nil {
		return err
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if secrets.Delete(cleanup, a.CredentialRef) != nil {
		return llm.ErrSecretCleanup
	}
	return nil
}
