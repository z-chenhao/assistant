package app

import (
	"context"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/task"
)

func (a *App) TaskFeedbacks(ctx context.Context, taskRef, before string) (task.FeedbackPage, error) {
	return a.store.TaskFeedbacks(ctx, taskRef, before)
}

func (a *App) RespondTaskFeedback(ctx context.Context, ref, answer string) (agent.Receipt, error) {
	if !a.Actor(nil).Ready(ctx) {
		return agent.Receipt{}, agent.ErrSetup
	}
	return a.runtimeRuns().RespondTaskFeedback(ctx, ref, answer)
}
