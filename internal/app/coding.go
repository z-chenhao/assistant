package app

import (
	"context"

	"github.com/z-chenhao/assistant/internal/artifact"
	"github.com/z-chenhao/assistant/internal/coding"
)

func (a *App) CodeReport(ctx context.Context, ref string, withDiff bool) (coding.Report, error) {
	return a.store.CodeReport(ctx, ref, withDiff)
}

func (a *App) SaveCodeReport(ctx context.Context, ref string) (artifact.Descriptor, error) {
	return a.store.SaveCodeReport(ctx, ref)
}
