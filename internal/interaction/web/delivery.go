package web

import (
	"context"
	"encoding/json"

	"github.com/z-chenhao/erii/internal/delivery"
	"github.com/z-chenhao/erii/internal/delivery/ui"
)

type ProjectionStore interface {
	DeliveryDestinationAllowed(context.Context, delivery.Request) error
	SaveProjection(context.Context, string, string, int, any) (json.RawMessage, error)
	ChannelReceipt(context.Context, string, int, string) (string, error)
	AcceptChannelReceipt(context.Context, string, int, string, string) error
}
type Adapter struct{ Store ProjectionStore }

func (Adapter) Idempotent() bool { return true }
func (a Adapter) Query(ctx context.Context, e delivery.Envelope) delivery.Observation {
	if a.Store.DeliveryDestinationAllowed(ctx, e.Request) != nil {
		return delivery.Observation{Outcome: delivery.Failed, Code: "rejected"}
	}
	hash := ui.Hash(e.Message)
	id, err := a.Store.ChannelReceipt(ctx, e.Request.DeliveryID, 0, hash)
	if err != nil {
		return delivery.Observation{Outcome: delivery.Unknown}
	}
	if id == "" {
		return delivery.Observation{Outcome: delivery.Retry}
	}
	return delivery.Observation{Outcome: delivery.Delivered}
}
func (a Adapter) Send(ctx context.Context, e delivery.Envelope) delivery.Observation {
	if o := a.Query(ctx, e); o.Outcome != delivery.Retry {
		return o
	}
	if _, err := delivery.Compose(e.Request, e.Message); err != nil || e.Request.Channel != "web" {
		return delivery.Observation{Outcome: delivery.Failed, Code: "rejected"}
	}
	hash := ui.Hash(e.Message)
	if _, err := a.Store.SaveProjection(ctx, e.Request.DeliveryID, hash, ui.Version, ui.Compose(e)); err != nil {
		return delivery.Observation{Outcome: delivery.Retry}
	}
	if a.Store.AcceptChannelReceipt(ctx, e.Request.DeliveryID, 0, hash, e.Message.MessageID) != nil {
		return delivery.Observation{Outcome: delivery.Unknown}
	}
	return delivery.Observation{Outcome: delivery.Delivered}
}
