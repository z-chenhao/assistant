package llm

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SecretStore is consumed only by the trusted authentication boundary. Backend
// errors must never escape it: OS errors may contain credential material.
type SecretStore interface {
	Get(context.Context, string) ([]byte, error)
	Put(context.Context, string, []byte) error
	Delete(context.Context, string) error
}

var (
	ErrSecretStore   = errors.New("credential store unavailable")
	ErrSecretCleanup = errors.New("credential cleanup failed; reconcile the unreferenced credential")
)

type AuthInput struct {
	Provider       string     `json:"provider"`
	AuthKind       string     `json:"auth_kind"`
	AccountLabel   string     `json:"account_label,omitempty"`
	WorkspaceLabel string     `json:"workspace_label,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

type AuthConnection struct {
	AuthInput
	ConnectionID string `json:"connection_id"`
	// Never serialize the credential locator into diagnostics, events, or snapshots.
	CredentialRef   string     `json:"-"`
	Status          string     `json:"status"`
	LastRefreshedAt *time.Time `json:"last_refreshed_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	Version         int64      `json:"version"`
}

func (a AuthInput) Validate() error {
	if strings.TrimSpace(a.Provider) == "" || (a.AuthKind != "api_key" && a.AuthKind != "chatgpt_oauth") || (a.AuthKind == "chatgpt_oauth" && a.Provider != "openai") {
		return errors.New("invalid authentication provider or auth_kind")
	}
	return nil
}

type AuthWriter interface {
	InsertAuth(context.Context, AuthConnection) error
}

// RegisterAuth writes a new secret before its metadata reference. A failed
// database transaction removes only this newly generated, unreferenced secret.
// Account OAuth uses its separate validated lifecycle; this helper registers keys.
func RegisterAuth(ctx context.Context, store AuthWriter, secrets SecretStore, input AuthInput, secret []byte) (AuthConnection, error) {
	if err := input.Validate(); err != nil {
		return AuthConnection{}, err
	}
	if len(secret) == 0 {
		return AuthConnection{}, errors.New("empty credential")
	}
	id, err := uuid.NewV7()
	if err != nil {
		return AuthConnection{}, errors.New("create authentication identity")
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if input.ExpiresAt != nil {
		expires := input.ExpiresAt.UTC().Truncate(time.Microsecond)
		input.ExpiresAt = &expires
	}
	a := AuthConnection{AuthInput: input, ConnectionID: id.String(), CredentialRef: "credential:" + id.String(), Status: "connected", CreatedAt: now, UpdatedAt: now, Version: 1}
	if err := secrets.Put(ctx, a.CredentialRef, secret); err != nil {
		return AuthConnection{}, ErrSecretStore
	}
	if err := store.InsertAuth(ctx, a); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if cleanupErr := secrets.Delete(cleanupCtx, a.CredentialRef); cleanupErr != nil {
			return AuthConnection{}, errors.Join(ErrStorage, ErrSecretCleanup)
		}
		return AuthConnection{}, ErrStorage
	}
	return a, nil
}
