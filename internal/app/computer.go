package app

import (
	"context"

	"github.com/z-chenhao/erii/internal/computer"
)

// ComputerStatus is a read-only operator diagnostic. Lifecycle decisions remain
// inside the accepted Code Mode invocation and are never CLI/model VM commands.
func (a *App) ComputerStatus(ctx context.Context) (computer.Status, error) {
	return a.store.ComputerStatus(ctx)
}
