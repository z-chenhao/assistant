package app

import (
	"context"

	"github.com/z-chenhao/assistant/internal/evidence"
)

func (a *App) ReadEvidence(ctx context.Context, q evidence.Request) (evidence.Page, error) {
	return a.runtimeRuns().ReadEvidence(ctx, q)
}
