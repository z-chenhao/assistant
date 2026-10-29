package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/z-chenhao/collie/internal/observability"
)

const traceFields = executionColumns + `,r.conversation_id,coalesce(substr(t.title,1,160),'Main conversation'),m.model_id,
 coalesce((SELECT substr(json_extract(i.message_json,'$.content[0].text'),1,160) FROM main_input i WHERE i.run_id=r.run_id ORDER BY i.sequence LIMIT 1),substr(t.title,1,160),''),
 coalesce((SELECT substr(json_extract(g.result_json,'$.text'),1,160) FROM model_generation g WHERE g.run_id=r.run_id AND g.status='completed' AND NOT EXISTS(SELECT 1 FROM context_compaction c WHERE c.run_id=g.run_id AND c.step_index=g.step_index AND json_extract(c.provenance_json,'$.stage')='summary_request') ORDER BY g.step_index DESC LIMIT 1),'')`
const traceFrom = ` FROM run r LEFT JOIN task t ON t.task_id=r.task_id JOIN model_snapshot m ON m.snapshot_id=r.model_snapshot_id WHERE r.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1) AND (r.kind='main' OR r.worker_position='root')`

func scanTrace(row scanner) (observability.Trace, error) {
	var t observability.Trace
	var created int64
	var started, ended sql.NullInt64
	err := row.Scan(&t.ID, &t.Role, &t.Status, &t.WaitReason, &t.Parent, &t.Root, &t.Depth, &t.Epoch, &t.TaskRef, &created, &started, &ended, &t.Session, &t.Title, &t.Model, &t.Input, &t.Output)
	t.CreatedAt = stamp(created)
	if started.Valid {
		t.StartedAt = stamp(started.Int64)
	}
	if ended.Valid {
		t.EndedAt = stamp(ended.Int64)
	}
	return t, err
}

func traceRoot(ctx context.Context, tx *sql.Tx, id string) (observability.Trace, error) {
	var root string
	err := tx.QueryRowContext(ctx, `SELECT CASE WHEN kind='main' THEN run_id ELSE root_run_id END FROM run WHERE run_id=? AND agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1)`, id).Scan(&root)
	if errors.Is(err, sql.ErrNoRows) {
		return observability.Trace{}, observability.ErrNotFound
	}
	if err != nil {
		return observability.Trace{}, err
	}
	t, err := scanTrace(tx.QueryRowContext(ctx, `SELECT `+traceFields+traceFrom+` AND r.run_id=?`, root))
	if errors.Is(err, sql.ErrNoRows) {
		err = observability.ErrNotFound
	}
	return t, err
}

func (s Inspector) Traces(ctx context.Context, q observability.TraceQuery) (observability.Traces, error) {
	out := observability.Traces{Items: []observability.Trace{}}
	if err := q.Validate(); err != nil {
		return out, err
	}
	c, err := readCursor(q.Cursor, "traces:"+q.Session)
	if err != nil {
		return out, err
	}
	query := `SELECT ` + traceFields + traceFrom
	args := []any{}
	if q.Session != "" {
		query += ` AND r.conversation_id=?`
		args = append(args, q.Session)
	}
	if q.Role != "" {
		query += ` AND CASE WHEN r.kind='main' THEN 'main' ELSE r.worker_position END=?`
		args = append(args, q.Role)
	}
	if q.Status != "" {
		query += ` AND r.status=?`
		args = append(args, q.Status)
	}
	if !q.Since.IsZero() {
		query += ` AND r.created_at>=?`
		args = append(args, q.Since.UnixMicro())
	}
	if !q.Until.IsZero() {
		query += ` AND r.created_at<?`
		args = append(args, q.Until.UnixMicro())
	}
	if c.ID != "" {
		query += ` AND (r.created_at,r.run_id)<(?,?)`
		args = append(args, c.At, c.ID)
	}
	query += ` ORDER BY r.created_at DESC,r.run_id DESC LIMIT ?`
	limit := pageLimit(q.Limit)
	args = append(args, limit+1)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		t, e := scanTrace(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		if len(out.Items) == limit {
			last := out.Items[limit-1]
			at, _ := parseStamp(last.CreatedAt)
			out.Next = nextCursor("traces:"+q.Session, at, last.ID)
			break
		}
		out.Items = append(out.Items, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for i := range out.Items {
		out.Items[i].Metrics, err = traceMetrics(ctx, tx, out.Items[i].ID)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

// Only unique accepted generations contribute usage/cost. Reuse and HTTP
// retries never multiply these ledgers; absent usage/pricing remain unknown.
func traceMetrics(ctx context.Context, tx *sql.Tx, id string) (observability.Metrics, error) {
	m := observability.Metrics{Costs: []observability.Cost{}}
	var ms sql.NullInt64
	const scope = ` FROM model_generation g JOIN run r USING(run_id) LEFT JOIN provider_usage u USING(generation_id) LEFT JOIN spend_reservation s USING(generation_id) WHERE (r.run_id=? OR r.root_run_id=?)`
	err := tx.QueryRowContext(ctx, `SELECT count(*),count(u.input_tokens),coalesce(sum(u.input_tokens),0),coalesce(sum(u.output_tokens),0),coalesce(sum(u.cache_read_tokens),0),coalesce(sum(u.cache_write_tokens),0),sum(u.latency_ms),count(*)-count(s.actual_micros)`+scope, id, id).Scan(&m.Generations, &m.KnownUsage, &m.Input, &m.Output, &m.CacheRead, &m.CacheWrite, &ms, &m.Unpriced)
	if err != nil {
		return m, err
	}
	if ms.Valid {
		m.ProviderMS = &ms.Int64
	}
	rows, err := tx.QueryContext(ctx, `SELECT s.currency,sum(s.actual_micros)`+scope+` AND s.status='priced' GROUP BY s.currency LIMIT 20`, id, id)
	// Parenthesize scope for the cost filter (Main's own rows obey it too).
	if err != nil {
		return m, err
	}
	defer rows.Close()
	for rows.Next() {
		var c observability.Cost
		if err = rows.Scan(&c.Currency, &c.Micros); err != nil {
			return m, err
		}
		m.Costs = append(m.Costs, c)
	}
	return m, rows.Err()
}

func scopeObservationsSQL() string {
	return `WITH members AS (SELECT * FROM run WHERE (run_id=? OR root_run_id=?) AND agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1)), observations AS (` + strings.Join(observationSources, " UNION ALL ") + `) `
}

func scanObservation(row scanner) (observability.Observation, error) {
	var o observability.Observation
	var step, end sql.NullInt64
	err := row.Scan(&o.ID, &o.Parent, &o.Type, &o.Name, &o.RunID, &o.Key, &step, &o.Status, &o.Started, &end, &o.Timing, &o.Detail)
	if step.Valid {
		o.Step = &step.Int64
	}
	if end.Valid {
		o.Ended = &end.Int64
		if o.Timing == "observed wait" {
			o.Status = "resumed"
		}
	}
	return o, err
}

func (s Inspector) Observations(ctx context.Context, q observability.ObservationQuery) (observability.ObservationTree, error) {
	out := observability.ObservationTree{Items: []observability.Observation{}}
	if err := q.Validate(); err != nil {
		return out, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	out.Trace, err = traceRoot(ctx, tx, q.RunID)
	if err != nil {
		return out, err
	}
	id := out.Trace.ID
	out.Trace.Metrics, err = traceMetrics(ctx, tx, id)
	if err != nil {
		return out, err
	}
	c, err := readCursor(q.Cursor, "observations:"+id)
	if err != nil {
		return out, err
	}
	query := scopeObservationsSQL() + `SELECT * FROM observations`
	args := []any{id, id}
	if c.ID != "" {
		query += ` WHERE (started,id)>(?,?)`
		args = append(args, c.At, c.ID)
	}
	query += ` ORDER BY started,id LIMIT 101`
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		o, e := scanObservation(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		if len(out.Items) == 100 {
			last := out.Items[99]
			out.Next = nextCursor("observations:"+id, last.Started, last.ID)
			break
		}
		out.Items = append(out.Items, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if q.Selected == "" {
		q.Selected = "agent:" + q.RunID
	}
	out.Selected, err = scanObservation(tx.QueryRowContext(ctx, scopeObservationsSQL()+`SELECT * FROM observations WHERE id=?`, id, id, q.Selected))
	if errors.Is(err, sql.ErrNoRows) {
		err = observability.ErrNotFound
	}
	return out, err
}

func parseStamp(raw string) (int64, error) {
	at, err := time.Parse(time.RFC3339Nano, raw)
	return at.UnixMicro(), err
}
