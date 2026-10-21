package app

import (
	"context"

	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/delivery"
	deliveryfake "github.com/z-chenhao/erii/internal/delivery/fake"
)

// SetTestDelivery is explicit process-local failure injection. The Fake wraps
// the same durable terminal receipt boundary used by the production adapter.
func (a *App) SetTestDelivery(mode string) error {
	switch mode {
	case "retry-once", "unavailable", "ambiguous", "unknown":
		a.testDelivery = mode
		return nil
	}
	return delivery.ErrConflict
}
func (a *App) Deliver(ctx context.Context) error {
	if a.eventsRequired.Load() && !a.eventsReady.Load() {
		return nil
	}
	if err := a.Runs().Flush(ctx); err != nil {
		return err
	}
	d, err := a.store.Diagnostics(ctx)
	if err != nil {
		return err
	}
	var adapter delivery.Adapter = a.terminalInbox
	if a.testDelivery != "" {
		// Retain Fake attempt state across terminal polls in this process.
		if a.deliveryFake == nil {
			a.deliveryFake = &deliveryfake.Adapter{Base: adapter, Mode: a.testDelivery}
		}
		adapter = a.deliveryFake
	}
	if err := a.deliverChannels(ctx); err != nil {
		return err
	}
	return (delivery.Service{Store: a.store.Deliveries(), History: a.history, Adapter: adapter, Channel: "tui", Destination: d.AgentID, Policy: a.deliveryPolicy, Boundary: func(name string) {
		if a.testBoundary != nil {
			a.testBoundary(name)
		}
	}}).Drain(ctx)
}
func (a *App) DeliveryRequests(ctx context.Context) ([]delivery.Request, error) {
	d, err := a.store.Diagnostics(ctx)
	if err != nil {
		return nil, err
	}
	return a.store.Deliveries().List(ctx, d.AgentID)
}

// TerminalHistory displays assistant messages only after the channel accepted
// them. Explicit /history may redisplay the retained projection. User input is
// visible immediately, including while a delivery is waiting for recovery.
func (a *App) TerminalHistory(ctx context.Context) ([]conversation.Message, error) {
	if err := a.Deliver(ctx); err != nil {
		return nil, err
	}
	d, err := a.store.Diagnostics(ctx)
	if err != nil {
		return nil, err
	}
	messages, err := a.history.List(ctx, d.AgentID)
	if err != nil {
		return nil, err
	}
	requests, err := a.store.Deliveries().List(ctx, d.AgentID)
	if err != nil {
		return nil, err
	}
	return a.terminalInbox.History(ctx, d.AgentID, messages, requests)
}
