package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/z-chenhao/erii/internal/agent"
)

// stopTask persists intent before any executor is signalled. The Task epoch
// immediately fences business writes; the old owner retains only stop/lease
// authority until its current step has joined. No model result is salvaged
// after this boundary. Already committed results and checkpoints are retained.
func (r *Runs) stopTask(ctx context.Context, tx *sql.Tx, taskID, status string, epoch, version int64) error {
	now := time.Now().UnixMicro()
	var cancelled any
	if status == "cancelled" {
		cancelled = now
	}
	res, err := tx.ExecContext(ctx, `UPDATE task SET status=?,task_epoch=task_epoch+1,cancelled_at=?,updated_at=?,version=version+1 WHERE task_id=? AND version=? AND task_epoch=?`, status, cancelled, now, taskID, version, epoch)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return agent.ErrConflict
	}
	reason := "task_paused"
	if status == "cancelled" {
		reason = "task_cancelled"
	}
	// Every current tree member is covered, including future descendants. No
	// second cancellation pipeline is needed for child-bearing invocations.
	if _, err = tx.ExecContext(ctx, `UPDATE run SET desired_state=?,updated_at=?,version=version+1 WHERE task_id=? AND status IN ('queued','running','waiting')`, reason, now, taskID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE tool_invocation SET desired_state='cancelled',updated_at=?,version=version+1 WHERE run_id IN (SELECT run_id FROM run WHERE task_id=?) AND status IN ('queued','running','waiting') AND desired_state!='cancelled'`, now, taskID); err != nil {
		return err
	}
	r.host.boundary("before_stop_commit")
	return r.recoverStopsTx(ctx, tx)
}

func ownership(ctx context.Context, tx *sql.Tx, c agent.RunClaim) error {
	owner, ok := c.(*claim)
	if !ok {
		return agent.ErrClaimLost
	}
	var valid int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM run_claim c JOIN run r USING(run_id) WHERE c.run_id=? AND c.token=? AND c.epoch=? AND c.expires_at>? AND r.status='running'`, owner.id, owner.token, owner.epoch, time.Now().UnixMicro()).Scan(&valid); err != nil {
		return err
	}
	if valid != 1 {
		return agent.ErrClaimLost
	}
	return nil
}

func (r *Runs) StopRequested(ctx context.Context, c agent.RunClaim) (bool, error) {
	owner, ok := c.(*claim)
	if !ok {
		return false, agent.ErrClaimLost
	}
	stop := false
	err := r.host.db.QueryRowContext(ctx, `SELECT r.desired_state!='running' FROM run r JOIN run_claim c USING(run_id) WHERE r.run_id=? AND c.token=? AND c.epoch=? AND c.expires_at>? AND r.status='running'`, owner.id, owner.token, owner.epoch, time.Now().UnixMicro()).Scan(&stop)
	if err == sql.ErrNoRows {
		return false, agent.ErrClaimLost
	}
	if err != nil {
		return false, agent.ErrStorage
	}
	return stop, nil
}

// SettleStop is called only after execution has joined. It cannot be used by
// an old/replaced owner to acknowledge another executor's work.
func (r *Runs) SettleStop(ctx context.Context, c agent.RunClaim) (bool, error) {
	stopped := false
	err := r.write(ctx, func(tx *sql.Tx) error {
		stopped = false
		if err := ownership(ctx, tx, c); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT desired_state!='running' FROM run WHERE run_id=?`, c.RunID()).Scan(&stopped); err != nil || !stopped {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE run_claim SET token=NULL,expires_at=0,version=version+1 WHERE run_id=?`, c.RunID()); err != nil {
			return err
		}
		return r.recoverStopsTx(ctx, tx)
	})
	return stopped, err
}

func (r *Runs) recoverStops(ctx context.Context) error {
	return r.write(ctx, func(tx *sql.Tx) error { return r.recoverStopsTx(ctx, tx) })
}

// Cancel leaves before parents. Live owners must join first; after a crash the
// lease is the fence. A stopped Run is never claimed to perform business work.
func (r *Runs) recoverStopsTx(ctx context.Context, tx *sql.Tx) error {
	for {
		var id, reason string
		err := tx.QueryRowContext(ctx, `SELECT r.run_id,r.desired_state FROM run r JOIN run_claim c USING(run_id) WHERE r.desired_state!='running' AND r.status IN ('queued','running','waiting') AND (c.token IS NULL OR c.expires_at<=?) AND NOT EXISTS(SELECT 1 FROM run child WHERE child.parent_run_id=r.run_id AND child.status IN ('queued','running','waiting')) ORDER BY r.depth DESC,r.created_at,r.run_id LIMIT 1`, time.Now().UnixMicro()).Scan(&id, &reason)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		now := time.Now().UnixMicro()
		if err = invalidateApprovals(ctx, tx, id); err != nil {
			return err
		}
		changed, err := tx.ExecContext(ctx, `UPDATE model_generation SET status='failed',error_code='GENERATION_CANCELLED',completed_at=? WHERE run_id=? AND status='pending'`, now, id)
		if err != nil {
			return err
		}
		if n, _ := changed.RowsAffected(); n > 0 {
			if err = runEvent(ctx, tx, id, "model_generation.failed", 1); err != nil {
				return err
			}
		}
		// Dispatched unsafe executions lack authoritative outcomes after their
		// owner is gone. Unexecuted approval/resource waiters can cancel safely.
		rows, err := tx.QueryContext(ctx, `UPDATE tool_invocation SET status=CASE WHEN attempt_count=0 OR replay_policy='safe' THEN 'cancelled' ELSE 'unknown' END,wait_reason=NULL,result_ready_at=?,updated_at=?,version=version+1 WHERE run_id=? AND status IN ('queued','running','waiting') RETURNING status,version`, now, now, id)
		if err != nil {
			return err
		}
		type transition struct {
			status  string
			version int64
		}
		transitions := []transition{}
		for rows.Next() {
			var item transition
			if err = rows.Scan(&item.status, &item.version); err != nil {
				rows.Close()
				return err
			}
			transitions = append(transitions, item)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, item := range transitions {
			if err = runEvent(ctx, tx, id, "tool_invocation."+item.status, item.version); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, `UPDATE run SET status='cancelled',wait_reason=NULL,error_code=?,ended_at=?,updated_at=?,version=version+1 WHERE run_id=?`, reason, now, now, id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE run_claim SET token=NULL,expires_at=0,version=version+1 WHERE run_id=?`, id); err != nil {
			return err
		}
		if err = runEvent(ctx, tx, id, "run.cancelled", 1); err != nil {
			return err
		}
		r.host.boundary("before_stop_settle_commit")
	}
}
