// Package secret owns OS credentials and explicit environment references.
package secret

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/z-chenhao/collie/internal/llm"
)

type Store struct{}

func (Store) Get(ctx context.Context, ref string) ([]byte, error) {
	if name, ok := strings.CutPrefix(ref, "env:"); ok {
		if !ValidName(name) {
			return nil, llm.ErrSecretStore
		}
		value, ok := os.LookupEnv(name)
		if !ok || value == "" {
			return nil, llm.ErrSecretStore
		}
		return []byte(value), nil
	}
	return get(ctx, ref)
}
func (Store) Put(ctx context.Context, ref string, value []byte) error { return put(ctx, ref, value) }
func (Store) Delete(ctx context.Context, ref string) error {
	if strings.HasPrefix(ref, "env:") {
		return nil
	}
	return remove(ctx, ref)
}
func ValidName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	for i, c := range name {
		if !(c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// RegisterEnvironment persists only an explicit environment reference.
func RegisterEnvironment(ctx context.Context, writer llm.AuthWriter, name string) (llm.AuthConnection, error) {
	if !ValidName(name) {
		return llm.AuthConnection{}, llm.ErrSecretStore
	}
	value, err := (Store{}).Get(ctx, "env:"+name)
	if err != nil {
		return llm.AuthConnection{}, llm.ErrSecretStore
	}
	for i := range value {
		value[i] = 0
	}
	id, err := uuid.NewV7()
	if err != nil {
		return llm.AuthConnection{}, llm.ErrStorage
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	connection := llm.AuthConnection{AuthInput: llm.AuthInput{Provider: "openai_compatible", AuthKind: "api_key"}, ConnectionID: id.String(), CredentialRef: "env:" + name, Status: "connected", CreatedAt: now, UpdatedAt: now, Version: 1}
	return connection, writer.InsertAuth(ctx, connection)
}
