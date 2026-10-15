// Package app is the composition root. Readiness and persistence belong to agent;
// process configuration and telemetry belong to their respective modules.
package app

import (
	"context"
	"errors"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/agent/sqlite"
	"github.com/z-chenhao/erii/internal/config"
	"github.com/z-chenhao/erii/internal/event"
	eventnats "github.com/z-chenhao/erii/internal/event/nats"
	llmsqlite "github.com/z-chenhao/erii/internal/llm/sqlite"
	"github.com/z-chenhao/erii/internal/observability"
)

type App struct {
	store     *sqlite.Store
	telemetry *observability.Telemetry
}

func Open(ctx context.Context, cfg config.Config) (*App, error) {
	t, err := observability.Open(cfg.App.DataRoot, cfg.LogLevel())
	if err != nil {
		return nil, err
	}
	ctx, startup, err := t.Start(ctx, "app.startup")
	if err != nil {
		return nil, errors.Join(err, t.Close())
	}
	_, database, err := t.Start(ctx, "sqlite.open_and_migrate")
	if err != nil {
		startup.End(err)
		return nil, errors.Join(err, t.Close())
	}
	store, err := sqlite.Open(ctx, cfg.App.DataRoot)
	database.End(err)
	if err != nil {
		startup.End(err)
		return nil, errors.Join(err, t.Close())
	}
	startup.Logger.Info("application started")
	startup.End(nil)
	return &App{store: store, telemetry: t}, nil
}

func (a *App) Diagnostics(ctx context.Context) (agent.Diagnostics, error) {
	return a.store.Diagnostics(ctx)
}

func (a *App) Models() *llmsqlite.Store { return a.store.Models() }

func (a *App) Close() error {
	err := a.store.Close()
	a.telemetry.Logger.Info("application stopped")
	return errors.Join(err, a.telemetry.Close())
}

// Relay runs infrastructure only. It never enables Agent ingress or a model.
func (a *App) Relay(ctx context.Context, cfg config.Config) error {
	return event.Relay(ctx, a.store.Events(), func(ctx context.Context) (event.EventBus, error) {
		bus, err := eventnats.Open(ctx, cfg.NATS)
		if err != nil {
			return nil, err
		}
		return bus, nil
	}, cfg.NATS.RetryInterval, a.telemetry.Logger)
}
