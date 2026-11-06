package app

import (
	"context"
	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/delivery/ui"
	"github.com/z-chenhao/assistant/internal/interaction/web"
)

func (a *App) WebUIState(ctx context.Context, ref string) (ui.ViewState, error) {
	if err := a.CheckChannel(ctx, "web", web.Subject, web.Destination); err != nil {
		return ui.ViewState{}, err
	}
	return a.store.WebUIState(ctx, ref)
}
func (a *App) SaveWebUIState(ctx context.Context, ref string, state ui.ViewState) (ui.ViewState, error) {
	if err := a.CheckChannel(ctx, "web", web.Subject, web.Destination); err != nil {
		return state, err
	}
	return a.store.SaveWebUIState(ctx, ref, state)
}
func (a *App) RequestWebUIAction(ctx context.Context, ref, source string) (agent.Receipt, error) {
	if err := a.CheckChannel(ctx, "web", web.Subject, web.Destination); err != nil {
		return agent.Receipt{}, err
	}
	core := a.core.Load()
	d, err := a.Diagnostics(ctx)
	if err != nil || !d.AgentIngressEnabled || core.registry == nil || a.core.Load() != core {
		return agent.Receipt{}, agent.ErrSetup
	}
	return core.runs.SubmitUIAction(ctx, ref, source)
}
func (a *App) WebUIActionResult(ctx context.Context, ref, source string) (ui.ActionResult, error) {
	if err := a.CheckChannel(ctx, "web", web.Subject, web.Destination); err != nil {
		return ui.ActionResult{}, err
	}
	return a.store.WebUIActionResult(ctx, ref, source)
}
