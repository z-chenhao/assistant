package sqlite

import (
	"context"
	"database/sql"
	"github.com/z-chenhao/assistant/internal/observability"
)

// WorkspaceWorkers sorts by execution time rather than task creation or the
// time of the latest prose update. Running-only reads cannot be crowded out by
// a busy history. Both views are bounded and scoped to the configured agent.
func (s Inspector) WorkspaceWorkers(ctx context.Context, runningOnly bool) (observability.WorkerActivityList, error) {
	out := observability.WorkerActivityList{Items: []observability.WorkerActivity{}}
	query := `SELECT r.run_id,coalesce(substr(t.title,1,160),'Worker'),r.worker_position,r.status,coalesce(t.task_ref,''),r.created_at,r.started_at,r.ended_at` + executionFrom + ` AND r.kind='worker'`
	if runningOnly {
		query += ` AND r.status='running'`
	}
	query += ` ORDER BY coalesce(r.started_at,r.created_at) DESC,r.run_id DESC LIMIT 51`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v observability.WorkerActivity
		var created int64
		var started, ended sql.NullInt64
		if err = rows.Scan(&v.ID, &v.Title, &v.Role, &v.Status, &v.TaskRef, &created, &started, &ended); err != nil {
			return out, err
		}
		if len(out.Items) == 50 {
			out.More = true
			break
		}
		v.CreatedAt = stamp(created)
		if started.Valid {
			v.StartedAt = stamp(started.Int64)
		}
		if ended.Valid {
			v.EndedAt = stamp(ended.Int64)
		}
		out.Items = append(out.Items, v)
	}
	return out, rows.Err()
}
