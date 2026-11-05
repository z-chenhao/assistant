package sqliteindex

import (
	"context"
	"database/sql"
	"math"
	"strings"
	"time"

	"github.com/z-chenhao/assistant/internal/recall"
)

// SearchLocal uses the existing text index without binding or changing a vector
// space. Turning semantic search on later can safely index the same originals.
func SearchLocal(ctx context.Context, db *sql.DB, scope recall.Scope, q recall.Query, source string) ([]recall.Hit, error) {
	if !q.Valid() || scope.UserID == "" || scope.AgentID == "" {
		return nil, recall.ErrInvalid
	}
	start, end := int64(math.MinInt64), int64(math.MaxInt64)
	if q.TimeRange != nil {
		if q.TimeRange.Start != nil {
			start = q.TimeRange.Start.UnixMicro()
		}
		if q.TimeRange.End != nil {
			end = q.TimeRange.End.UnixMicro()
		}
	}
	term := words(q.Query)
	out := []recall.Hit{}
	if term == "" {
		return out, nil
	}
	// instr also matches a Chinese phrase within unicode61's unsegmented text.
	rows, err := db.QueryContext(ctx, `SELECT d.ref,d.content,d.occurred_at,d.author_role FROM recall_document d
WHERE d.user_id=? AND (d.agent_id='' OR d.agent_id=?) AND d.active=1 AND d.occurred_at>=? AND d.occurred_at<=?
AND (d.rowid IN (SELECT rowid FROM recall_fts WHERE recall_fts MATCH ?) OR instr(lower(d.content),?)>0)
ORDER BY d.core DESC,d.occurred_at DESC,d.ref LIMIT ?`, scope.UserID, scope.AgentID, start, end, term, strings.ToLower(strings.TrimSpace(q.Query)), q.Count())
	if err != nil {
		return nil, recall.ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var h recall.Hit
		var at int64
		if rows.Scan(&h.SourceRef, &h.Content, &at, &h.AuthorRole) != nil {
			return nil, recall.ErrUnavailable
		}
		h.SourceType, h.OccurredAt = source, time.UnixMicro(at).UTC()
		out = append(out, h)
	}
	if rows.Err() != nil {
		return nil, recall.ErrUnavailable
	}
	return out, nil
}
