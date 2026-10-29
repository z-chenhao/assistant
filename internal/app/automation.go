package app

import (
	"context"
	"errors"
	"time"

	"github.com/z-chenhao/collie/internal/agent"
	"github.com/z-chenhao/collie/internal/automation"
	"github.com/z-chenhao/collie/internal/tool"
)

func (a *App) ManageAutomation(ctx context.Context, key string, q automation.Request) (tool.Result, error) {
	return a.runtimeRuns().ManageAutomation(ctx, key, q, a.runtimeRuns().AutomationNow())
}
func (a *App) ScheduleAutomations(ctx context.Context) error {
	return a.runtimeRuns().ScheduleAutomations(ctx, a.runtimeRuns().AutomationNow())
}
func (a *App) SetTestAutomationClock(now time.Time) {
	a.testAutomationClock = &now
	a.runtimeRuns().SetTestAutomationClock(now)
}

// WorkAutomations is a foreground lifecycle adapter. Reasoning and execution
// remain in Actor.Drain; independent CLI/TUI ingress shares its claims.
func (a *App) WorkAutomations(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := a.Actor(nil).Drain(ctx); err != nil && !errors.Is(err, agent.ErrSetup) {
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
