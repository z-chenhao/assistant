package app

import (
	"context"
	"errors"

	"github.com/z-chenhao/assistant/internal/observer/github"
)

func (a *App) GitHubStatus(ctx context.Context) (github.Status, error) {
	if a.github == nil {
		return github.Status{State: "not_configured"}, nil
	}
	return a.store.GitHubStatus(ctx, a.github.Config)
}

func (a *App) PollGitHub(ctx context.Context) error {
	if a.github == nil {
		return errors.New("github configuration is required")
	}
	return a.github.Poll(ctx)
}

func (a *App) SetGitHubWatch(ctx context.Context, enabled bool) error {
	if a.github == nil {
		return errors.New("github configuration is required")
	}
	return a.store.SetGitHubWatch(ctx, a.github.Config, enabled)
}

func (a *App) GitHubObservations(ctx context.Context, before int64) ([]github.Observation, error) {
	if a.github == nil {
		return nil, errors.New("github configuration is required")
	}
	return a.store.GitHubObservations(ctx, a.github.Config, before)
}
