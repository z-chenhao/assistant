package app

import (
	"context"
	"errors"
	"github.com/z-chenhao/erii/internal/automation"
	"github.com/z-chenhao/erii/internal/tool"
	"time"
)

func (a *App) ManageAutomation(ctx context.Context, key string, q automation.Request) (tool.Result, error) {
	return a.runs.ManageAutomation(ctx, key, q, a.runs.AutomationNow())
}
func (a *App) ScheduleAutomations(ctx context.Context) error {
	return a.runs.ScheduleAutomations(ctx, a.runs.AutomationNow())
}
func (a *App) SetTestAutomationClock(now time.Time) { a.runs.SetTestAutomationClock(now) }

// WorkAutomations is a foreground lifecycle adapter. Reasoning and execution
// remain in Actor.Drain; independent CLI/TUI ingress shares its claims.
func (a *App) WorkAutomations(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := a.Actor(nil).Drain(ctx); err != nil {
			if ctx.Err() != nil && errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
