package sqlite

import (
	"context"
	"database/sql"

	"github.com/z-chenhao/assistant/internal/task"
)

// Scheduled includes saved intent before its first fire. It does not invent a
// Task or admit work, and follows Automation's existing privacy-epoch boundary.
func (r *Reader) Scheduled(ctx context.Context, query string) ([]task.ScheduledWork, error) {
	if len(query) > 300 {
		return nil, task.ErrInvalid
	}
	rows, err := r.db.QueryContext(ctx, `SELECT a.automation_id,coalesce(t.task_ref,''),a.instruction,a.trigger_type,a.trigger_spec,a.timezone,a.version,a.enabled,a.next_fire_at,a.below_value,
 a.intent_privacy_epoch=coalesce((SELECT epoch FROM memory_privacy WHERE agent_id=a.agent_id),0)
 FROM automation a LEFT JOIN task t ON t.task_id=a.task_id AND t.agent_id=a.agent_id
 WHERE a.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1)
 AND (?='' OR (a.intent_privacy_epoch=coalesce((SELECT epoch FROM memory_privacy WHERE agent_id=a.agent_id),0) AND instr(lower(a.instruction),lower(?))>0))
 ORDER BY a.enabled DESC,a.next_fire_at IS NULL,a.next_fire_at,a.created_at,a.automation_id LIMIT 50`, query, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []task.ScheduledWork{}
	for rows.Next() {
		v, e := scanScheduled(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func scanScheduled(row scanner) (task.ScheduledWork, error) {
	var v task.ScheduledWork
	var next, below sql.NullInt64
	var visible bool
	if err := row.Scan(&v.Ref, &v.TaskRef, &v.Instruction, &v.Type, &v.Spec, &v.Timezone, &v.Version, &v.Enabled, &next, &below, &visible); err != nil {
		return v, err
	}
	if !visible {
		v.Schedule = task.Schedule{Ref: v.Ref, Version: v.Version, Enabled: v.Enabled, Restricted: true}
	} else {
		if next.Valid {
			v.NextAt = stamp(next.Int64)
		}
		if below.Valid {
			value := below.Int64
			v.Below = &value
		}
	}
	return v, nil
}

// ScheduledDetail reads an owner-owned schedule even when it is outside the
// bounded list. Revoked context remains unavailable to the editor.
func (r *Reader) ScheduledDetail(ctx context.Context, ref string) (task.ScheduledWork, error) {
	if len(ref) > 256 {
		return task.ScheduledWork{}, task.ErrInvalid
	}
	v, err := scanScheduled(r.db.QueryRowContext(ctx, `SELECT a.automation_id,coalesce(t.task_ref,''),a.instruction,a.trigger_type,a.trigger_spec,a.timezone,a.version,a.enabled,a.next_fire_at,a.below_value,
 a.intent_privacy_epoch=coalesce((SELECT epoch FROM memory_privacy WHERE agent_id=a.agent_id),0)
 FROM automation a LEFT JOIN task t ON t.task_id=a.task_id AND t.agent_id=a.agent_id
 WHERE a.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1) AND a.automation_id=?`, ref))
	if err == sql.ErrNoRows {
		return v, task.ErrNotFound
	}
	return v, err
}
