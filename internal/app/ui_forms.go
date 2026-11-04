package app

import (
	"context"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/conversation"
	"github.com/z-chenhao/assistant/internal/delivery/ui"
	"github.com/z-chenhao/assistant/internal/interaction"
	"github.com/z-chenhao/assistant/internal/interaction/web"
)

// RespondWebForm accepts ordinary user input. The client never supplies the
// prompt, labels, approval identities, target Run, or idempotency key.
func (a *App) RespondWebForm(ctx context.Context, messageID, formID string, values map[string]string) (agent.Receipt, error) {
	if err := a.CheckChannel(ctx, "web", web.Subject, web.Destination); err != nil {
		return agent.Receipt{}, err
	}
	d, err := a.store.Diagnostics(ctx)
	if err != nil {
		return agent.Receipt{}, err
	}
	m, err := a.history.Get(ctx, d.AgentID, messageID)
	if err != nil {
		return agent.Receipt{}, err
	}
	if m.Role != "assistant" {
		return agent.Receipt{}, conversation.ErrConflict
	}
	// Use the accepted projection, including old projections that intentionally
	// remain text-only. Never reinterpret a client-provided component definition.
	raw, err := a.store.WebProjection(ctx, m)
	if err != nil {
		return agent.Receipt{}, err
	}
	c, err := ui.Form(raw, messageID, formID)
	if err != nil {
		return agent.Receipt{}, err
	}
	content, err := ui.Answer(c, values)
	if err != nil {
		return agent.Receipt{}, err
	}
	source := "ui-form:" + formID
	return a.acceptInbound(ctx, interaction.InboundEvent{Channel: "web", Subject: web.Subject, Destination: web.Destination, EventID: source, SourceID: source, Content: content})
}
