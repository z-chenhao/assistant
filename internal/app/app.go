// Package app is the composition root. Readiness and persistence belong to agent;
// process configuration and telemetry belong to their respective modules.
package app

import (
	"context"
	"errors"
	"os"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/agent/sqlite"
	"github.com/z-chenhao/erii/internal/config"
	"github.com/z-chenhao/erii/internal/event"
	eventnats "github.com/z-chenhao/erii/internal/event/nats"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/llm/bridge"
	"github.com/z-chenhao/erii/internal/llm/fake"
	"github.com/z-chenhao/erii/internal/llm/secret"
	llmsqlite "github.com/z-chenhao/erii/internal/llm/sqlite"
	"github.com/z-chenhao/erii/internal/observability"
)

type App struct {
	store     *sqlite.Store
	telemetry *observability.Telemetry
	provider  *bridge.Client
	control   llm.Control
}

func Open(ctx context.Context, cfg config.Config) (*App, error) {
	return open(ctx, cfg, "")
}

// OpenWithTestBridge is explicit test injection. No config default, environment
// discovery, missing credential, or saved active profile can select a Fake.
func OpenWithTestBridge(ctx context.Context, cfg config.Config, fixture string) (*App, error) {
	if !fake.ValidFixture(fixture) {
		return nil, llm.InvalidProviderResponse
	}
	return open(ctx, cfg, fixture)
}

func open(ctx context.Context, cfg config.Config, fixture string) (*App, error) {
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
	a := &App{store: store, telemetry: t}
	a.control = llm.Control{Store: store.Models(), Secrets: secret.Store{}}
	if fixture != "" {
		path, err := os.Executable()
		if err == nil {
			a.provider, err = bridge.Open(ctx, path, "-test-provider-bridge", fixture)
		}
		if err != nil {
			a.control.BridgeError = llm.NormalizeError(err.Error())
		} else {
			a.control.Probe = a.provider
			a.control.BridgeIdentity = "test-bridge-v1:" + fixture
		}
	}
	if fixture == "" && cfg.LLM.ProviderBridgeCommand != "" {
		a.provider, err = bridge.OpenLocal(ctx, cfg.LLM.ProviderBridgeCommand, cfg.LLM.ProviderBridgeScript, store.Models().Resolver(a.control.Secrets), store.Models().RecordUsage)
		if err != nil {
			a.control.BridgeError = llm.NormalizeError(err.Error())
		} else {
			a.control.Probe = a.provider
			a.control.BridgeIdentity = cfg.LLM.ProviderBridgeVersion
		}
	}
	startup.Logger.Info("application started")
	startup.End(nil)
	return a, nil
}

func (a *App) Diagnostics(ctx context.Context) (agent.Diagnostics, error) {
	d, err := a.store.Diagnostics(ctx)
	if err != nil {
		return d, err
	}
	return agent.EffectiveReadiness(ctx, d, a.control, a.provider), nil
}

func (a *App) Models() *llmsqlite.Store  { return a.store.Models() }
func (a *App) ModelControl() llm.Control { return a.control }
func (a *App) Catalog(ctx context.Context) ([]llm.CatalogModel, error) {
	if a.provider == nil {
		if a.control.BridgeError != nil {
			return nil, a.control.BridgeError
		}
		return nil, llm.ErrNoBridge
	}
	return a.provider.List(ctx)
}

func (a *App) Close() error {
	if a.provider != nil {
		_ = a.provider.Close()
	}
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

func (a *App) CatalogProfile(ctx context.Context, id string) ([]llm.CatalogModel, error) {
	if a.provider == nil {
		return nil, llm.ErrNoBridge
	}
	p, err := a.Models().Profile(ctx, id)
	if err != nil {
		return nil, err
	}
	return a.provider.CatalogForProfile(ctx, p)
}

// LoginKey consumes only stdin bytes; credentials never enter arguments or metadata.
func (a *App) LoginKey(ctx context.Context, value []byte) (llm.AuthConnection, error) {
	return llm.RegisterAuth(ctx, a.Models(), a.control.Secrets, llm.AuthInput{Provider: "openai_compatible", AuthKind: "api_key"}, value)
}
func (a *App) LoginEnvironment(ctx context.Context, name string) (llm.AuthConnection, error) {
	return secret.RegisterEnvironment(ctx, a.Models(), name)
}

func (a *App) Discover(ctx context.Context, endpoint, authID string) ([]llm.CatalogModel, error) {
	if a.provider == nil {
		return nil, llm.ErrNoBridge
	}
	connection, err := a.Models().Connection(ctx, endpoint, authID, a.control.Secrets)
	if err != nil {
		return nil, err
	}
	return a.provider.CatalogAt(ctx, connection)
}
