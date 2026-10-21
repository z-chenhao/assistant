package feishu

import (
	"context"
	"encoding/json"
	"io"

	"github.com/z-chenhao/erii/internal/artifact"
	"github.com/z-chenhao/erii/internal/delivery"
	"github.com/z-chenhao/erii/internal/delivery/ui"
)

type DeliveryStore interface {
	DeliveryDestinationAllowed(context.Context, delivery.Request) error
	SaveProjection(context.Context, string, string, int, any) (json.RawMessage, error)
	ChannelReceipt(context.Context, string, int, string) (string, error)
	AcceptChannelReceipt(context.Context, string, int, string, string) error
}
type Artifacts interface {
	DescribeArtifact(context.Context, string) (artifact.Descriptor, error)
	OpenArtifact(context.Context, string) (io.ReadCloser, error)
}
type Adapter struct {
	Client    Client
	Store     DeliveryStore
	Artifacts Artifacts
}

// Feishu's UUID dedup window is bounded. A crashed, uncertain remote send is
// query-only after restart; it never acquires permission for another send.
func (Adapter) Idempotent() bool { return false }
func (a Adapter) Query(ctx context.Context, e delivery.Envelope) delivery.Observation {
	return a.deliver(ctx, e, false)
}
func (a Adapter) Send(ctx context.Context, e delivery.Envelope) delivery.Observation {
	return a.deliver(ctx, e, true)
}
func (a Adapter) deliver(ctx context.Context, e delivery.Envelope, send bool) delivery.Observation {
	fail := delivery.Observation{Outcome: delivery.Unknown}
	if a.Store.DeliveryDestinationAllowed(ctx, e.Request) != nil {
		return delivery.Observation{Outcome: delivery.Failed, Code: "rejected"}
	}
	if _, err := delivery.Compose(e.Request, e.Message); err != nil || e.Request.Channel != "feishu" {
		return delivery.Observation{Outcome: delivery.Failed, Code: "rejected"}
	}
	spec := ui.Compose(e)
	raw, err := a.Store.SaveProjection(ctx, e.Request.DeliveryID, ui.Hash(e.Message), ui.Version, spec)
	if err != nil {
		return fail
	}
	if json.Unmarshal(raw, &spec) != nil {
		spec = ui.Spec{SchemaVersion: ui.Version, Fallback: ui.Compose(e).Fallback}
	}
	// Each part has a durable receipt under the original DeliveryRequest. Crash
	// before/after an upload may leave an orphan resource, never a repeated notice.
	parts := []struct {
		kind    string
		content any
		ref     string
	}{{kind: "interactive", content: ui.Card(spec)}}
	for _, p := range e.Message.Content {
		if p.ArtifactRef != "" {
			parts = append(parts, struct {
				kind    string
				content any
				ref     string
			}{kind: "file", ref: p.ArtifactRef})
		}
	}
	for i, p := range parts {
		hash := ui.Hash([]any{e.Request.DestinationRef, p.kind, p.content, p.ref})
		remote, err := a.Store.ChannelReceipt(ctx, e.Request.DeliveryID, i, hash)
		if err != nil {
			return fail
		}
		if remote != "" {
			continue
		}
		if !send {
			return fail
		}
		content := p.content
		if p.ref != "" {
			d, err := a.Artifacts.DescribeArtifact(ctx, p.ref)
			if err != nil {
				return delivery.Observation{Outcome: delivery.Failed, Code: "rejected"}
			}
			src, err := a.Artifacts.OpenArtifact(ctx, p.ref)
			if err != nil {
				return fail
			}
			key, err := a.Client.Upload(ctx, d, src)
			_ = src.Close()
			if err != nil {
				return fail
			}
			content = map[string]string{"file_key": key}
		}
		remote, err = a.Client.Send(ctx, e.Request.DestinationRef, keyFor(e.Request.DeliveryID, i), p.kind, content)
		if err != nil {
			return fail
		}
		if a.Store.AcceptChannelReceipt(ctx, e.Request.DeliveryID, i, hash, remote) != nil {
			return fail
		}
	}
	return delivery.Observation{Outcome: delivery.Delivered}
}
