//go:build (!darwin && !linux) || !cgo

package secret

import (
	"context"

	"github.com/z-chenhao/assistant/internal/llm"
)

func get(context.Context, string) ([]byte, error) { return nil, llm.ErrSecretStore }
func put(context.Context, string, []byte) error   { return llm.ErrSecretStore }
func remove(context.Context, string) error        { return llm.ErrSecretStore }

func NativeBackend() (string, bool) { return "OS credential store (native cgo build required)", false }
