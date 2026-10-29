package sqlite

import (
	"context"
	"database/sql"
	"strings"

	"github.com/z-chenhao/collie/internal/agent"
	"github.com/z-chenhao/collie/internal/artifact"
	"github.com/z-chenhao/collie/internal/memory"
	"github.com/z-chenhao/collie/internal/recall"
)

// ManageMemory is the authenticated local control path. Model calls use the
// same queue/revocation transaction through their accepted ToolInvocation.
func (r *Runs) ManageMemory(ctx context.Context, key string, m memory.Mutation) (map[string]any, error) {
	if r.memory == nil {
		return nil, recall.ErrUnavailable
	}
	raw, _ := encoded(m)
	if _, ok := memory.Parse(raw); !ok || key == "" || len(key) > 128 {
		return nil, recall.ErrInvalid
	}
	id := ""
	ref := "control:" + key
	payload := raw
	if m.Content != "" && (r.vault.Ref != "" || memory.Sensitive(m.Content)) {
		var e error
		payload, e = r.vault.Seal(ctx, raw, ref)
		if e != nil {
			return nil, e
		}
	}
	e := r.write(ctx, func(tx *sql.Tx) error {
		var agentID string
		if e := tx.QueryRowContext(ctx, `SELECT agent_id FROM agent_config WHERE singleton=1`).Scan(&agentID); e != nil {
			return e
		}
		var prior string
		e := tx.QueryRowContext(ctx, `SELECT event_id,payload FROM memory_mutation WHERE source_type='feedback' AND source_ref=?`, ref).Scan(&id, &prior)
		if e == nil {
			if strings.HasPrefix(prior, "sealed:v1:") {
				prior, e = r.vault.Open(ctx, prior, ref)
				if e != nil {
					return e
				}
			}
			if prior != raw {
				return agent.ErrConflict
			}
			return nil
		}
		if e != sql.ErrNoRows {
			return e
		}
		check := m.MemoryRef
		if check == "" {
			check = m.Supersedes
		}
		if check != "" {
			if e := r.memory.CheckRef(ctx, recall.Scope{UserID: artifact.LocalUser, AgentID: agentID}, strings.TrimPrefix(check, "memory:"), m.Operation == "forget"); e != nil {
				return e
			}
		}
		id, e = r.queueMemory(ctx, tx, agentID, m.Operation, "feedback", ref, payload)
		if e != nil {
			return e
		}
		if m.Operation == "forget" {
			return r.revokeMemory(ctx, tx, agentID)
		}
		return nil
	})
	return map[string]any{"accepted": e == nil, "operation": m.Operation, "receipt": "memory-event:" + id}, e
}

func (r *Runs) MemoryStatus(ctx context.Context) ([]map[string]any, error) {
	rows, e := r.host.db.QueryContext(ctx, `SELECT event_id,operation,applied_at,attempts,failed_at,last_error FROM memory_mutation ORDER BY sequence`)
	if e != nil {
		return nil, agent.ErrStorage
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, op string
		var at, failed sql.NullInt64
		var code sql.NullString
		var attempts int
		if rows.Scan(&id, &op, &at, &attempts, &failed, &code) != nil {
			return nil, agent.ErrStorage
		}
		out = append(out, map[string]any{"receipt": "memory-event:" + id, "operation": op, "applied": at.Valid, "attempts": attempts, "rejected": failed.Valid, "last_error": code.String})
	}
	return out, rows.Err()
}
