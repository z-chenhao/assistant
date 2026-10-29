// Package fake supplies deterministic transport failures around an independent
// durable receipt store. Explicit injection only; never a default provider.
package fake

import (
	"context"
	"sync"

	"github.com/z-chenhao/collie/internal/delivery"
)

type Adapter struct {
	Base   delivery.Adapter
	Mode   string
	mu     sync.Mutex
	failed map[string]bool
}

func (a *Adapter) Idempotent() bool { return a.Mode != "unknown" && a.Base.Idempotent() }

func (a *Adapter) Send(ctx context.Context, e delivery.Envelope) delivery.Observation {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.failed == nil {
		a.failed = map[string]bool{}
	}
	switch a.Mode {
	case "retry-once":
		if !a.failed[e.Request.DeliveryID] {
			a.failed[e.Request.DeliveryID] = true
			return delivery.Observation{Outcome: delivery.Retry, Code: "unavailable"}
		}
	case "unavailable":
		return delivery.Observation{Outcome: delivery.Retry, Code: "unavailable"}
	case "unknown":
		return delivery.Observation{Outcome: delivery.Unknown}
	case "ambiguous":
		a.Base.Send(ctx, e)
		return delivery.Observation{Outcome: delivery.Unknown}
	}
	return a.Base.Send(ctx, e)
}
func (a *Adapter) Query(ctx context.Context, e delivery.Envelope) delivery.Observation {
	if a.Mode == "unknown" {
		return delivery.Observation{Outcome: delivery.Unknown}
	}
	return a.Base.Query(ctx, e)
}
