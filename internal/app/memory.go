package app

import (
	"context"

	"github.com/z-chenhao/erii/internal/memory"
)

func (a *App) ManageMemory(ctx context.Context, key string, m memory.Mutation) (map[string]any, error) {
	return a.runtimeRuns().ManageMemory(ctx, key, m)
}
func (a *App) CurateMemory(ctx context.Context) error { return a.runtimeRuns().CurateMemory(ctx) }
func (a *App) MemoryStatus(ctx context.Context) ([]map[string]any, error) {
	return a.runtimeRuns().MemoryStatus(ctx)
}

func (a *App) StartMemory(ctx context.Context) {
	if a.memoryCurator == nil {
		return
	}
	a.memoryOnce.Do(func() {
		work, cancel := context.WithCancel(ctx)
		a.memoryCancel = cancel
		a.memoryDone = make(chan struct{})
		go func() { defer close(a.memoryDone); a.memoryCurator.Run(work) }()
	})
}
