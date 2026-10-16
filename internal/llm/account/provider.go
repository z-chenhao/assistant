package account

import (
	"context"
	"errors"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/llm/bridge"
)

func (m *Manager) Check(ctx context.Context, id string) (int64, error) {
	a, _, err := m.Ensure(ctx, id)
	return a.Version, err
}

func (m *Manager) Resolver(local bridge.Resolver) bridge.Resolver {
	return func(ctx context.Context, snapshot llm.Snapshot) (bridge.Connection, error) {
		if snapshot.ProviderKind != "openai_account" {
			return local(ctx, snapshot)
		}
		if snapshot.WireAPI != "openai_responses" || snapshot.EndpointHash != "" {
			return bridge.Connection{}, llm.ModelIncompatible
		}
		id := snapshot.AuthConnectionID
		registration, err := m.Store.Registration(ctx, id)
		if err != nil {
			return bridge.Connection{}, llm.AuthRequired
		}
		a, c, err := m.Ensure(ctx, id)
		if err != nil {
			return bridge.Connection{}, err
		}
		guard := func(ctx context.Context) error {
			if ctx.Err() != nil {
				return llm.GenerationCancelled
			}
			a, err := m.Store.Auth(ctx, id)
			if ctx.Err() != nil {
				return llm.GenerationCancelled
			}
			if err != nil {
				return llm.ProviderUnavailable
			}
			if a.Status != "connected" && a.Status != "refreshing" {
				return llm.AuthRequired
			}
			current, err := m.Store.Registration(ctx, id)
			if ctx.Err() != nil {
				return llm.GenerationCancelled
			}
			if err != nil {
				return llm.ProviderUnavailable
			}
			if current.AuthEpoch != registration.AuthEpoch {
				return llm.AuthRequired
			}
			return nil
		}
		if err = guard(ctx); err != nil {
			return bridge.Connection{}, err
		}
		return bridge.Connection{Endpoint: "https://api.openai.com/v1", AccessToken: c.AccessToken, ConnectionID: id, AuthVersion: a.Version, AuthEpoch: registration.AuthEpoch, Guard: guard}, nil
	}
}
func (m *Manager) Observer(record func(context.Context, llm.GenerationRequest, llm.GenerationResult, time.Duration, error) error) bridge.Observer {
	return func(ctx context.Context, r llm.GenerationRequest, c bridge.Connection, result llm.GenerationResult, latency time.Duration, cause error) error {
		if r.Snapshot.ProviderKind == "openai_account" {
			if errors.Is(cause, llm.AuthRequired) && c.AuthVersion > 0 {
				_ = m.LoseVersion(ctx, r.Snapshot.AuthConnectionID, c.AuthVersion)
			}
			if c.Guard != nil {
				if guardErr := c.Guard(ctx); guardErr != nil {
					cause = guardErr
					if err := record(ctx, r, llm.GenerationResult{}, latency, cause); err != nil {
						return err
					}
					return cause
				}
			}
		}
		return record(ctx, r, result, latency, cause)
	}
}
func (m *Manager) Catalog(ctx context.Context, id string, list func(context.Context, bridge.Connection) ([]llm.CatalogModel, error)) ([]llm.CatalogModel, error) {
	c, err := m.Resolver(nil)(ctx, llm.Snapshot{ProviderKind: "openai_account", WireAPI: "openai_responses", AuthConnectionID: id})
	if err != nil {
		return nil, err
	}
	models, err := list(ctx, c)
	if errors.Is(err, llm.AuthRequired) {
		_ = m.LoseVersion(ctx, id, c.AuthVersion)
	}
	if guardErr := c.Guard(ctx); guardErr != nil {
		return nil, guardErr
	}
	return models, err
}
