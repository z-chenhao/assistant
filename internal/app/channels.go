package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/artifact"
	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/delivery"
	"github.com/z-chenhao/erii/internal/interaction"
	"github.com/z-chenhao/erii/internal/interaction/feishu"
	"github.com/z-chenhao/erii/internal/interaction/web"
)

func (a *App) BindChannel(ctx context.Context, c, s, d string, enabled bool, v int64) (interaction.Binding, error) {
	return a.store.BindChannel(ctx, c, s, d, enabled, v)
}
func (a *App) ChannelBindings(ctx context.Context) ([]interaction.Binding, error) {
	return a.store.ChannelBindings(ctx)
}
func (a *App) CheckChannel(ctx context.Context, c, s, d string) error {
	return a.store.CheckChannel(ctx, c, s, d)
}
func (a *App) AcceptInbound(ctx context.Context, e interaction.InboundEvent) (agent.Receipt, error) {
	if !a.Actor(nil).Ready(ctx) {
		return agent.Receipt{}, agent.ErrSetup
	}
	return a.runs.AcceptInbound(ctx, e)
}
func (a *App) DecideInbound(ctx context.Context, e interaction.InboundEvent, id, hash, decision string) error {
	return a.runs.DecideInbound(ctx, e, id, hash, decision)
}
func (a *App) WebStatus(ctx context.Context) (web.Status, error) {
	d, err := a.Diagnostics(ctx)
	if err != nil {
		return web.Status{}, err
	}
	status, err := a.store.AgentStatus(ctx)
	return web.Status{Readiness: d.Readiness, Status: status, ConfigVersion: d.ConfigVersion}, err
}
func (a *App) ImportChannelArtifact(ctx context.Context, e interaction.InboundEvent, key string, input artifact.Input, src io.Reader) (string, error) {
	return a.store.ImportChannelArtifact(ctx, e, key, input, src)
}
func (a *App) SaveWebProjection(ctx context.Context, m conversation.Message) (json.RawMessage, error) {
	return a.store.WebProjection(ctx, m)
}
func (a *App) ServeWeb(ctx context.Context, listener net.Listener, cfg web.Config) error {
	s := web.Server{Config: cfg, Core: a, Models: a.Models(), Control: a.control, Accounts: a.accounts, Secrets: a.control.Secrets, AccountCatalog: a.AccountCatalog}
	if a.feishu != nil {
		s.Callback = feishu.Handler{Client: *a.feishu, Core: a, Wake: func() {
			if s.Wake != nil {
				s.Wake()
			}
		}}
	}
	// The server owns ingress only. A single foreground lifecycle coordinates
	// Actor draining, Automation, delivery recovery and orderly shutdown.
	return web.Run(ctx, &s, listener, func(ctx context.Context) error {
		err := a.Actor(nil).Drain(ctx)
		if errors.Is(err, agent.ErrSetup) {
			return nil
		}
		return err
	}, a.Deliver)
}
func (a *App) deliverChannels(ctx context.Context) error {
	bindings, err := a.store.ChannelBindings(ctx)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, b := range bindings {
		key := b.Channel + ":" + b.Destination
		if seen[key] {
			continue
		}
		seen[key] = true
		var adapter delivery.Adapter = web.Adapter{Store: a.store}
		if b.Channel == "feishu" {
			if a.feishu == nil {
				continue
			}
			adapter = feishu.Adapter{Client: *a.feishu, Store: a.store, Artifacts: a}
		}
		if err := (delivery.Service{Store: a.store.Deliveries(), History: a.history, Adapter: adapter, Channel: b.Channel, Destination: b.Destination, Policy: a.deliveryPolicy, Boundary: a.testBoundary}).Drain(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) ManageChannel(ctx context.Context, op, channel, subject, destination string, enabled bool, version int64, binding string, out io.Writer) error {
	switch op {
	case "list":
		v, e := a.store.ChannelBindings(ctx)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(v)
	case "bind":
		v, e := a.store.BindChannel(ctx, channel, subject, destination, enabled, version)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(v)
	case "preference":
		v, e := a.store.ChannelPreference(ctx)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(v)
	case "prefer":
		if e := a.store.PreferChannel(ctx, channel, binding, version); e != nil {
			return e
		}
		v, e := a.store.ChannelPreference(ctx)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(v)
	}
	return agent.ErrConflict
}

func (a *App) ChannelPreference(ctx context.Context) (interaction.Preference, error) {
	return a.store.ChannelPreference(ctx)
}
func (a *App) PreferChannel(ctx context.Context, channel, binding string, version int64) error {
	return a.store.PreferChannel(ctx, channel, binding, version)
}
