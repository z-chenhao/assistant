package app

import (
	"context"

	"github.com/z-chenhao/collie/internal/agent"
	"github.com/z-chenhao/collie/internal/skill"
)

func (a *App) ReviewExperience(ctx context.Context, q skill.ReviewRequest) (map[string]any, error) {
	if a.skillConfig == nil {
		return nil, agent.ErrNotFound
	}
	return a.runtimeRuns().ReviewExperience(ctx, *a.skillConfig, q)
}
