package app

import (
	"context"
	"github.com/z-chenhao/assistant/internal/delivery/ui"
)

func (a *App) WebWidget(ctx context.Context, ref string) (ui.Widget, error) {
	return a.store.WebWidget(ctx, ref)
}
