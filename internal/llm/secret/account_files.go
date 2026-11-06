package secret

import (
	"context"
	"github.com/google/uuid"
	"github.com/z-chenhao/assistant/internal/llm"
	"strings"
)

// AccountFiles keeps the existing OAuth directory and UUID reference contract.
// The account manager serializes updates across processes.
type AccountFiles struct{ DataRoot string }

func accountSecretName(ref string) (string, error) {
	name, ok := strings.CutPrefix(ref, "credential:")
	id, err := uuid.Parse(name)
	if !ok || err != nil || id.String() != name {
		return "", llm.ErrSecretStore
	}
	return name, nil
}
func (s AccountFiles) Exists(ctx context.Context, ref string) (bool, error) {
	name, err := accountSecretName(ref)
	if err != nil {
		return false, err
	}
	return (fileStore{s.DataRoot, "account-secrets"}).Exists(ctx, name)
}
func (s AccountFiles) Get(ctx context.Context, ref string) ([]byte, error) {
	name, err := accountSecretName(ref)
	if err != nil {
		return nil, err
	}
	return (fileStore{s.DataRoot, "account-secrets"}).Get(ctx, name)
}
func (s AccountFiles) Put(ctx context.Context, ref string, value []byte) error {
	name, err := accountSecretName(ref)
	if err != nil {
		return err
	}
	return (fileStore{s.DataRoot, "account-secrets"}).Put(ctx, name, value)
}
func (s AccountFiles) Delete(ctx context.Context, ref string) error {
	name, err := accountSecretName(ref)
	if err != nil {
		return err
	}
	return (fileStore{s.DataRoot, "account-secrets"}).Delete(ctx, name)
}
