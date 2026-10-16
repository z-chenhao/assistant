// Package memory provides a deterministic SecretStore Fake for tests only.
package memory

import (
	"context"
	"errors"
	"sync"
)

var ErrMissing = errors.New("credential not found")

type SecretStore struct {
	mu     sync.Mutex
	values map[string][]byte
}

func New() *SecretStore { return &SecretStore{values: make(map[string][]byte)} }

func (s *SecretStore) Get(ctx context.Context, ref string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.values[ref]
	if !ok {
		return nil, ErrMissing
	}
	return append([]byte(nil), v...), nil
}

func (s *SecretStore) Put(ctx context.Context, ref string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[ref] = append([]byte(nil), value...)
	return nil
}

func (s *SecretStore) Delete(ctx context.Context, ref string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, ref)
	return nil
}
