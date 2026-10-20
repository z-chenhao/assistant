package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/z-chenhao/erii/internal/memory"
	"github.com/z-chenhao/erii/internal/recall"
	"strings"
	"time"
)

func (s *Store) AllowCategory(ctx context.Context, scope recall.Scope, category string, at time.Time, eventID string) error {
	if scope.UserID == "" || scope.AgentID == "" || !validCategory(category) || at.IsZero() || eventID == "" {
		return recall.ErrInvalid
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return recall.ErrUnavailable
	}
	defer tx.Rollback()
	var user, agent, oldCategory string
	var oldAt int64
	e = tx.QueryRowContext(ctx, `SELECT user_id,agent_id,category,accepted_at FROM memory_category_allow WHERE event_id=?`, eventID).Scan(&user, &agent, &oldCategory, &oldAt)
	if e == nil {
		if user != scope.UserID || agent != scope.AgentID || oldCategory != category || oldAt != at.UnixMicro() {
			return recall.ErrConflict
		}
		return nil
	}
	if e != sql.ErrNoRows {
		return recall.ErrUnavailable
	}
	if e = setPolicy(ctx, tx, scope, category, false, at); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO memory_category_allow VALUES(?,?,?,?,?)`, eventID, scope.UserID, scope.AgentID, category, at.UnixMicro()); e != nil {
		return recall.ErrUnavailable
	}
	if tx.Commit() != nil {
		return recall.ErrUnavailable
	}
	return nil
}

// Persist opt-out before deleting current records. A crash can leave only more
// tombstones to apply; it cannot allow new automatic records in the category.
func (s *Store) ForgetCategory(ctx context.Context, scope recall.Scope, category string, at time.Time, eventID string) error {
	if scope.UserID == "" || scope.AgentID == "" || at.IsZero() || eventID == "" {
		return recall.ErrInvalid
	}
	if !validCategory(category) {
		return recall.ErrInvalid
	}

	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return recall.ErrUnavailable
	}
	defer tx.Rollback()
	var user, agent, oldCategory, raw string
	var accepted int64
	e = tx.QueryRowContext(ctx, `SELECT user_id,agent_id,category,accepted_at,targets_json FROM memory_category_event WHERE event_id=?`, eventID).Scan(&user, &agent, &oldCategory, &accepted, &raw)
	var ids []string
	if e == nil {
		if user != scope.UserID || agent != scope.AgentID || oldCategory != category || accepted != at.UnixMicro() || json.Unmarshal([]byte(raw), &ids) != nil {
			return recall.ErrConflict
		}
	} else if e == sql.ErrNoRows {
		if e = setPolicy(ctx, tx, scope, category, true, at); e != nil {
			return e
		}
		rows, e := tx.QueryContext(ctx, `SELECT memory_id,user_id,scope_type,scope_id,content FROM memory_record WHERE user_id=? AND ((scope_type='user' AND scope_id=?) OR (scope_type='agent' AND scope_id=?)) AND deleted_at IS NULL`, scope.UserID, scope.UserID, scope.AgentID)
		if e != nil {
			return recall.ErrUnavailable
		}
		for rows.Next() {
			var r memory.Record
			if rows.Scan(&r.ID, &r.UserID, &r.ScopeType, &r.ScopeID, &r.Content) != nil {
				rows.Close()
				return recall.ErrUnavailable
			}
			text := r.Content
			if strings.HasPrefix(text, "sealed:v1:") {
				text, e = s.vault.Open(ctx, text, binding(r))
				if e != nil {
					rows.Close()
					return e
				}
			}
			if category == "all" || memory.Category(text) == category {
				ids = append(ids, r.ID)
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return recall.ErrUnavailable
		}
		rawBytes, _ := json.Marshal(ids)
		if _, e = tx.ExecContext(ctx, `INSERT INTO memory_category_event VALUES(?,?,?,?,?,?)`, eventID, scope.UserID, scope.AgentID, category, at.UnixMicro(), string(rawBytes)); e != nil {
			return recall.ErrUnavailable
		}
	} else {
		return recall.ErrUnavailable
	}
	if tx.Commit() != nil {
		return recall.ErrUnavailable
	}
	for _, id := range ids {
		if e = s.Forget(ctx, scope, id, at); e != nil {
			return e
		}
	}
	return nil
}

func setPolicy(ctx context.Context, tx *sql.Tx, scope recall.Scope, category string, blocked bool, at time.Time) error {
	rows, e := tx.QueryContext(ctx, `SELECT category,version FROM memory_policy WHERE user_id=? AND agent_id=? AND (?='all' OR category=?)`, scope.UserID, scope.AgentID, category, category)
	if e != nil {
		return recall.ErrUnavailable
	}
	type state struct {
		category string
		version  int64
	}
	var prior []state
	found := false
	for rows.Next() {
		var p state
		if rows.Scan(&p.category, &p.version) != nil {
			rows.Close()
			return recall.ErrUnavailable
		}
		prior = append(prior, p)
		found = found || p.category == category
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return recall.ErrUnavailable
	}
	for _, p := range prior {
		res, e := tx.ExecContext(ctx, `UPDATE memory_policy SET blocked=?,updated_at=?,version=version+1 WHERE user_id=? AND agent_id=? AND category=? AND version=?`, blocked, at.UnixMicro(), scope.UserID, scope.AgentID, p.category, p.version)
		if e != nil {
			return recall.ErrUnavailable
		}
		n, e := res.RowsAffected()
		if e != nil || n != 1 {
			return recall.ErrConflict
		}
	}
	if !found {
		if _, e = tx.ExecContext(ctx, `INSERT INTO memory_policy VALUES(?,?,?,?,?,?,1)`, scope.UserID, scope.AgentID, category, blocked, at.UnixMicro(), at.UnixMicro()); e != nil {
			return recall.ErrUnavailable
		}
	}
	return nil
}

func validCategory(category string) bool {
	for _, c := range []string{"health", "finance", "identity", "relationships", "goals", "learning", "preferences", "facts", "all"} {
		if category == c {
			return true
		}
	}
	return false
}
