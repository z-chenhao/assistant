package account

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/z-chenhao/assistant/internal/llm"
)

type cancelledReadStore struct {
	Store
	updated bool
}

func (s *cancelledReadStore) Auth(_ context.Context, id string) (llm.AuthConnection, error) {
	return llm.AuthConnection{
		AuthInput:    llm.AuthInput{AuthKind: "chatgpt_oauth"},
		ConnectionID: id, Status: "connected", Version: 1,
	}, nil
}

func (s *cancelledReadStore) AccountState(context.Context, llm.AuthConnection, string, *llm.OAuthCredential) error {
	s.updated = true
	return nil
}

type cancelledSecretRead struct {
	llm.SecretStore
	cancel  context.CancelFunc
	deleted bool
}

func (s *cancelledSecretRead) Get(context.Context, string) ([]byte, error) {
	s.cancel()
	return nil, llm.ErrSecretStore
}

func (s *cancelledSecretRead) Delete(context.Context, string) error {
	s.deleted = true
	return nil
}

func TestEnsureDoesNotRevokeAccountWhenCredentialReadIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &cancelledReadStore{}
	secrets := &cancelledSecretRead{cancel: cancel}
	manager := Manager{Store: store, Secrets: secrets, LockRoot: t.TempDir()}

	_, _, err := manager.Ensure(ctx, uuid.NewString())
	if !errors.Is(err, llm.ProviderUnavailable) {
		t.Fatalf("Ensure returned %v, want temporary unavailability", err)
	}
	if store.updated || secrets.deleted {
		t.Fatal("cancelled credential read changed account state or deleted the credential")
	}
}
