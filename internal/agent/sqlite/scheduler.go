package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/z-chenhao/collie/internal/agent"
)

func (r *Runs) ConfigureScheduler(l agent.SchedulerLimits) {
	r.scheduler = l
	r.host.scheduler.Store(&l)
}
func (s *Store) schedulerLimits() agent.SchedulerLimits {
	if l := s.scheduler.Load(); l != nil && l.Valid() {
		return *l
	}
	return agent.DefaultSchedulerLimits()
}
func (r *Runs) schedulerLimits() agent.SchedulerLimits {
	if r.scheduler.Valid() {
		return r.scheduler
	}
	return agent.DefaultSchedulerLimits()
}

// Queues are authoritative Run rows. A durable service ticket rotates among
// Root trees across processes/restarts; within a tree runnable descendants go
// first. Waiting ancestors consume no slots. No model or second queue API.
// A successor can accept immutable input while a revoked Main joins. It cannot
// execute until stop recovery has made every revoked Main terminal.
const mainPredecessorsJoined = `(r.kind!='main' OR NOT EXISTS(SELECT 1 FROM run prior WHERE prior.agent_id=r.agent_id AND prior.kind='main' AND prior.desired_state!='running' AND prior.status IN ('queued','running','waiting')))`

const runnable = mainPredecessorsJoined + ` AND NOT EXISTS(SELECT 1 FROM spend_approval sa WHERE sa.root_run_id=coalesce(r.root_run_id,r.run_id) AND sa.status='pending') AND r.desired_state='running' AND
 (r.kind='main' OR EXISTS(SELECT 1 FROM task t WHERE t.task_id=r.task_id AND t.task_epoch=r.task_epoch AND t.status='active')) AND
 (r.status IN ('queued','running') OR (r.status='waiting' AND
 ((r.wait_reason='user_input' AND
 (EXISTS(SELECT 1 FROM main_input i WHERE i.run_id=r.run_id AND i.sequence>json_extract(r.checkpoint_json,'$.input_sequence')) OR
 EXISTS(SELECT 1 FROM task_instruction i WHERE i.task_id=r.task_id AND i.task_epoch=r.task_epoch AND r.worker_position='root' AND i.sequence>json_extract(r.checkpoint_json,'$.input_sequence')))) OR
 (r.wait_reason='resource' AND (
 NOT EXISTS(SELECT 1 FROM tool_invocation i WHERE i.run_id=r.run_id AND i.status='waiting') OR
 EXISTS(SELECT 1 FROM tool_invocation i WHERE i.run_id=r.run_id AND i.status='waiting' AND
 (i.wait_reason='resource' OR (i.wait_reason='retry_backoff' AND i.next_wake_at<=:now) OR
 (i.wait_reason='approval' AND EXISTS(SELECT 1 FROM approval a WHERE a.invocation_id=i.invocation_id AND a.status!='pending')))) OR
 (EXISTS(SELECT 1 FROM tool_invocation i WHERE i.run_id=r.run_id AND i.status='waiting' AND i.wait_reason='child_run') AND
 NOT EXISTS(SELECT 1 FROM tool_invocation i JOIN agent_invocation a USING(invocation_id) JOIN run child ON child.run_id=a.child_run_id
 WHERE i.run_id=r.run_id AND i.status='waiting' AND i.wait_reason='child_run' AND child.status NOT IN ('completed','failed','cancelled'))))))))`

// Provider identity is the frozen account or endpoint, not the mutable profile.
const providerKey = `s.provider_kind||':'||coalesce(s.auth_connection_id,s.endpoint_hash,'local')`
const schedulerCTE = `WITH live AS (
 SELECT w.run_id,w.root_run_id,w.worker_position,w.agent_id,ws.provider_kind||':'||coalesce(ws.auth_connection_id,ws.endpoint_hash,'local') AS provider_key
 FROM run w JOIN run_claim wc USING(run_id) JOIN model_snapshot ws ON ws.snapshot_id=w.model_snapshot_id
 WHERE w.kind='worker' AND wc.token IS NOT NULL AND wc.expires_at>:now
) `
const capacity = `(r.kind='main' OR (
 (SELECT count(*) FROM live)<:global AND (SELECT count(*) FROM live)<:user AND
 (SELECT count(*) FROM live WHERE agent_id=r.agent_id)<:agent AND
 (SELECT count(*) FROM live WHERE provider_key=` + providerKey + `)<:provider AND
 (r.worker_position='root' OR (SELECT count(*) FROM live WHERE root_run_id=r.root_run_id AND worker_position='child')<:children) AND
 NOT EXISTS(SELECT 1 FROM scheduler_provider p WHERE p.provider_key=` + providerKey + ` AND p.blocked_until>:now)))`

func (r *Runs) scheduleArgs() []any {
	l := r.schedulerLimits()
	return []any{sql.Named("now", time.Now().UnixMicro()), sql.Named("global", l.Global), sql.Named("user", l.User), sql.Named("agent", l.Agent), sql.Named("provider", l.Provider), sql.Named("children", l.Children)}
}

const nextRunSQL = schedulerCTE + `SELECT r.run_id FROM run r JOIN run_claim c USING(run_id) JOIN model_snapshot s ON s.snapshot_id=r.model_snapshot_id LEFT JOIN scheduler_tree q ON q.root_run_id=r.root_run_id WHERE ` + runnable + ` AND (c.token IS NULL OR c.expires_at<=:now) AND ` + capacity + `
 ORDER BY CASE r.kind WHEN 'main' THEN 0 ELSE 1 END,coalesce(q.last_served,0),r.depth DESC,r.created_at,r.run_id LIMIT 1`

const activeRunSQL = `SELECT count(*) FROM run r WHERE (` + runnable + `) OR (r.desired_state!='running' AND r.status IN ('queued','running','waiting'))`

func (r *Runs) nextScheduled(ctx context.Context) (string, error) {
	var id string
	err := r.host.nextRun.QueryRowContext(ctx, r.scheduleArgs()...).Scan(&id)
	return id, err
}

func (r *Runs) claimCapacity(ctx context.Context, tx *sql.Tx, id string) error {
	args := append(r.scheduleArgs(), sql.Named("id", id))
	var allowed int
	if err := tx.QueryRowContext(ctx, schedulerCTE+`SELECT count(*) FROM run r JOIN model_snapshot s ON s.snapshot_id=r.model_snapshot_id WHERE r.run_id=:id AND `+capacity, args...).Scan(&allowed); err != nil {
		return err
	}
	if allowed != 1 {
		return agent.ErrBusy
	}
	return nil
}

// Infrastructure failure streaks are independent of business item failures.
// Usage/result acceptance and the breaker commit together; lost claims cannot
// change health. Cooldown gates future leases without failing queued work.
func (s *Store) recordProviderHealth(ctx context.Context, tx *sql.Tx, id, code string) error {
	l := s.schedulerLimits()
	var key string
	err := tx.QueryRowContext(ctx, `SELECT `+providerKey+` FROM run r JOIN model_snapshot s ON s.snapshot_id=r.model_snapshot_id WHERE r.run_id=? AND r.kind='worker'`, id).Scan(&key)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	infra := code == "PROVIDER_UNAVAILABLE" || code == "RATE_LIMITED"
	if !infra {
		_, err = tx.ExecContext(ctx, `INSERT INTO scheduler_provider(provider_key) VALUES(?) ON CONFLICT(provider_key) DO UPDATE SET consecutive_errors=0,blocked_until=0`, key)
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO scheduler_provider(provider_key,consecutive_errors) VALUES(?,1) ON CONFLICT(provider_key) DO UPDATE SET consecutive_errors=consecutive_errors+1,blocked_until=CASE WHEN consecutive_errors+1>=? THEN ? ELSE blocked_until END`, key, l.InfrastructureFailures, time.Now().Add(time.Duration(l.CooldownSeconds)*time.Second).UnixMicro())
	return err
}
