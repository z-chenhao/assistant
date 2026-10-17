package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/task"
)

// Progress records meaningful Host/Worker progress under its current claim. It
// creates neither TaskUpdate, Inbox, Outbox wake nor user delivery. Tool traces
// are not synthesized into progress. A later UI read observes this projection.
func (r *Runs) Progress(ctx context.Context, c agent.RunClaim, summary string) error {
	if strings.TrimSpace(summary) == "" || len(summary) > 512 {
		return agent.ErrConflict
	}
	return r.write(ctx, func(tx *sql.Tx) error {
		if err := fence(ctx, tx, c); err != nil {
			return err
		}
		var id string
		var version int64
		if err := tx.QueryRowContext(ctx, `SELECT t.task_id,t.version FROM task t JOIN run r USING(task_id) WHERE r.run_id=? AND r.kind='worker' AND r.worker_position='root'`, c.RunID()).Scan(&id, &version); err != nil {
			return agent.ErrConflict
		}
		res, err := tx.ExecContext(ctx, `UPDATE task SET progress_summary=?,updated_at=?,version=version+1 WHERE task_id=? AND version=?`, summary, time.Now().UnixMicro(), id, version)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return agent.ErrConflict
		}
		return nil
	})
}
func (r *Runs) Activity(ctx context.Context) ([]task.Summary, error) {
	rows, err := r.host.db.QueryContext(ctx, `SELECT task_ref,title,t.status,progress_summary,t.updated_at,
 (SELECT count(*) FROM run c WHERE c.task_id=t.task_id AND c.task_epoch=t.task_epoch AND c.worker_position='child' AND c.status='queued'),
 (SELECT count(*) FROM run c WHERE c.task_id=t.task_id AND c.task_epoch=t.task_epoch AND c.worker_position='child' AND c.status='running'),
 (SELECT count(*) FROM run c WHERE c.task_id=t.task_id AND c.task_epoch=t.task_epoch AND c.worker_position='child' AND c.status='completed'),
 (SELECT count(*) FROM run c WHERE c.task_id=t.task_id AND c.task_epoch=t.task_epoch AND c.worker_position='child' AND c.status IN ('failed','cancelled')),
 (SELECT count(*) FROM run c WHERE c.task_id=t.task_id AND c.task_epoch=t.task_epoch AND c.worker_position='child' AND c.status='waiting')
 FROM task t WHERE agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1) ORDER BY t.updated_at DESC,task_ref LIMIT 20`)
	if err != nil {
		return nil, agent.ErrStorage
	}
	defer rows.Close()
	out := []task.Summary{}
	for rows.Next() {
		var s task.Summary
		var at int64
		var queued, running, succeeded, failed, waiting int
		if err = rows.Scan(&s.TaskRef, &s.Title, &s.Status, &s.ProgressSummary, &at, &queued, &running, &succeeded, &failed, &waiting); err != nil {
			return nil, agent.ErrStorage
		}
		if queued+running+succeeded+failed+waiting > 0 {
			s.ProgressSummary = strings.TrimSpace(s.ProgressSummary + fmt.Sprintf(" Children: queued %d, running %d, succeeded %d, failed %d, waiting %d", queued, running, succeeded, failed, waiting))
		}
		s.UpdatedAt = time.UnixMicro(at).UTC().Format(time.RFC3339Nano)
		out = append(out, s)
	}
	if rows.Err() != nil {
		return nil, agent.ErrStorage
	}
	return out, nil
}
