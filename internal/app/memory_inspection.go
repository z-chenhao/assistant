package app

import (
	"context"
	"github.com/z-chenhao/assistant/internal/memory"
)

func (a *App) InspectMemory(ctx context.Context, q memory.InspectionQuery) (memory.InspectionPage, error) {
	return a.runtimeRuns().InspectMemory(ctx, q, a.memory)
}
