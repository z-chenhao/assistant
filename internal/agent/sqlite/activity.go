package sqlite

import (
	"context"
	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/task"
)

func (r *Runs) Activity(ctx context.Context) ([]task.Summary, error) {
	items, err := r.host.taskReader.Activity(ctx)
	if err != nil {
		return nil, agent.ErrStorage
	}
	return items, nil
}
