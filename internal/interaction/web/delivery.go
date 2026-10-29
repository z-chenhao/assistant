package web

import (
	"context"
	"encoding/json"

	"github.com/z-chenhao/collie/internal/delivery"
	"github.com/z-chenhao/collie/internal/delivery/ui"
	"github.com/z-chenhao/collie/internal/interaction"
)

type ProjectionStore interface {
	DeliveryBinding(context.Context, delivery.Request) (interaction.Binding, error)
	SaveProjection(context.Context, string, string, int, any) (json.RawMessage, error)
	ChannelReceipt(context.Context, string, int, string) (string, error)
	AcceptChannelReceipt(context.Context, string, int, string, string) error
}
type Adapter struct{ Store ProjectionStore }

func (Adapter) Idempotent() bool { return true }
func (a Adapter) Query(ctx context.Context, e delivery.Envelope) delivery.Observation {
	binding, err := a.Store.DeliveryBinding(ctx, e.Request)
	if err != nil || binding.Subject != Subject || binding.Destination != Destination {
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
