package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/task"
)

// Activity reads one consistent view of current-epoch execution facts. It does
// not interpret Tool traces, mutate Tasks, or create model/notification work.
func (r *Runs) Activity(ctx context.Context) ([]task.Summary, error) {
	rows, err := r.host.db.QueryContext(ctx, `WITH current_runs AS (
 SELECT r.run_id,r.task_id,r.worker_position,r.status,r.wait_reason,r.updated_at
 FROM run r JOIN task t ON t.task_id=r.task_id AND t.task_epoch=r.task_epoch
 WHERE t.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1)
), counts AS (
 SELECT task_id,max(updated_at) AS updated_at,
 sum(worker_position='child' AND status='queued') AS queued,
 sum(worker_position='child' AND status='running') AS running,
 sum(worker_position='child' AND status='completed') AS succeeded,
 sum(worker_position='child' AND status IN ('failed','cancelled')) AS failed,
 sum(worker_position='child' AND status='waiting') AS waiting,
 max(status='waiting' AND wait_reason='approval') AS approval,
 max(status='waiting' AND wait_reason='user_input') AS input
 FROM current_runs GROUP BY task_id
)
SELECT t.task_ref,t.title,t.status,t.progress_summary,max(t.updated_at,coalesce(c.updated_at,0)),
 coalesce(root.status,''),coalesce(root.wait_reason,''),
 coalesce(c.queued,0),coalesce(c.running,0),coalesce(c.succeeded,0),coalesce(c.failed,0),coalesce(c.waiting,0),
 (coalesce(c.approval,0) OR EXISTS(SELECT 1 FROM spend_approval a WHERE a.root_run_id=root.run_id AND a.status='pending')),coalesce(c.input,0),
 EXISTS(SELECT 1 FROM tool_invocation i JOIN current_runs r USING(run_id) WHERE r.task_id=t.task_id AND i.status='unknown')
FROM task t LEFT JOIN counts c USING(task_id)
 LEFT JOIN current_runs root ON root.task_id=t.task_id AND root.worker_position='root'
WHERE t.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1)
ORDER BY max(t.updated_at,coalesce(c.updated_at,0)) DESC,t.task_ref LIMIT 20`)
	if err != nil {
		return nil, agent.ErrStorage
	}
	defer rows.Close()
	out := []task.Summary{}
	for rows.Next() {
		var s task.Summary
		var at int64
		var state activityState
		if err = rows.Scan(&s.TaskRef, &s.Title, &s.Status, &s.ProgressSummary, &at,
			&state.root, &state.wait, &state.queued, &state.running, &state.succeeded, &state.failed, &state.waiting,
			&state.approval, &state.input, &state.unknown); err != nil {
			return nil, agent.ErrStorage
		}
		parts := []string{state.stage(s.Status)}
		if s.ProgressSummary != "" {
			parts = append(parts, "Last update: "+s.ProgressSummary)
		}
		if state.children() > 0 {
			parts = append(parts, fmt.Sprintf("Children: queued %d, running %d, succeeded %d, failed %d, waiting %d (%d/%d finished)", state.queued, state.running, state.succeeded, state.failed, state.waiting, state.succeeded+state.failed, state.children()))
		}
		s.ProgressSummary = strings.Join(parts, " ")
		s.UpdatedAt = time.UnixMicro(at).UTC().Format(time.RFC3339Nano)
		out = append(out, s)
	}
	if rows.Err() != nil {
		return nil, agent.ErrStorage
	}
	return out, nil
}

type activityState struct {
	root, wait                                  string
	queued, running, succeeded, failed, waiting int
	approval, input, unknown                    bool
}

func (s activityState) children() int {
	return s.queued + s.running + s.succeeded + s.failed + s.waiting
}

func (s activityState) stage(status string) string {
	switch status {
	case "paused":
		return "Paused."
	case "cancelled":
		return "Cancelled."
	case "completed":
		return "Completed."
	}
	if s.unknown {
		return "External action outcome is unconfirmed."
	}
	if s.approval {
		return "Waiting for approval."
	}
	if s.input {
		return "Waiting for your input."
	}
	switch s.root {
	case "queued":
		return "Queued."
	case "running":
		return "Working."
	case "failed":
		return "This attempt failed; the work remains open."
	case "cancelled":
		return "This attempt stopped; the work remains open."
	case "waiting", "completed":
		if s.queued+s.running+s.waiting > 0 {
			return "Waiting for delegated work."
		}
		if s.root == "completed" {
			return "This round is complete; the work remains open."
		}
		if s.wait == "external_event" {
			return "Waiting for an external event."
		}
		return "Waiting for available resources."
	default:
		return "Ready for the next request."
	}
}
