//go:build !darwin || !cgo

package secret

import (
	"context"

	"github.com/z-chenhao/erii/internal/llm"
)

func get(context.Context, string) ([]byte, error) { return nil, llm.ErrSecretStore }
func put(context.Context, string, []byte) error   { return llm.ErrSecretStore }
func remove(context.Context, string) error        { return llm.ErrSecretStore }
