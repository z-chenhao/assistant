package app

import (
	"context"
	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/skill"
)

func (a *App) ReviewExperience(ctx context.Context, q skill.ReviewRequest) (map[string]any, error) {
	if a.skillConfig == nil {
		return nil, agent.ErrNotFound
	}
	return a.runs.ReviewExperience(ctx, *a.skillConfig, q)
}
