package sqlite

import (
	"context"
	"strings"

	"github.com/z-chenhao/collie/internal/memory"
	"github.com/z-chenhao/collie/internal/recall"
)

// Representation migration: domain identity/provenance never change. Rewrite
// prior sensitive plaintext only under an exclusive write transaction, then
// rebuild derived FTS and remove old pages/WAL before enabling the provider.
func (s *Store) protectExisting(ctx context.Context) error {
	rows, e := s.db.QueryContext(ctx, `SELECT memory_id,user_id,scope_type,scope_id,content FROM memory_record WHERE content NOT LIKE 'sealed:v1:%'`)
	if e != nil {
		return recall.ErrUnavailable
	}
	var records []memory.Record
	for rows.Next() {
		var r memory.Record
		if rows.Scan(&r.ID, &r.UserID, &r.ScopeType, &r.ScopeID, &r.Content) != nil {
			rows.Close()
			return recall.ErrUnavailable
		}
		records = append(records, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return recall.ErrUnavailable
	}
	if len(records) == 0 {
		var pending int
		if s.db.QueryRowContext(ctx, `SELECT cleanup_pending FROM memory_protection WHERE singleton=1`).Scan(&pending) != nil {
			return recall.ErrUnavailable
		}
		if pending == 0 {
			return nil
		}
		return s.cleanupSealing(ctx)
	}
	if _, e = s.db.ExecContext(ctx, `PRAGMA secure_delete=ON`); e != nil {
		return recall.ErrUnavailable
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return recall.ErrUnavailable
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `UPDATE memory_protection SET cleanup_pending=1 WHERE singleton=1; DROP TRIGGER memory_immutable`); e != nil {
		return recall.ErrUnavailable
	}
	for _, r := range records {
		sealed, e := s.vault.Seal(ctx, r.Content, binding(r))
		if e != nil {
			return e
		}
		if strings.HasPrefix(r.Content, "sealed:") {
			return recall.ErrUnavailable
		}
		if _, e = tx.ExecContext(ctx, `UPDATE memory_record SET content=? WHERE memory_id=? AND content=?`, sealed, r.ID, r.Content); e != nil {
			return recall.ErrUnavailable
		}
		if _, e = tx.ExecContext(ctx, `UPDATE recall_document SET content='',vector_json=NULL,embedding_id=NULL,active=0 WHERE ref=?`, "memory:"+r.ID); e != nil {
			return recall.ErrUnavailable
		}
	}
	if _, e = tx.ExecContext(ctx, `CREATE TRIGGER memory_immutable BEFORE UPDATE OF memory_id,user_id,scope_type,scope_id,tier,content,source_refs,supersedes_memory_id,created_at ON memory_record BEGIN SELECT RAISE(ABORT,'immutable memory'); END; INSERT INTO recall_fts(recall_fts) VALUES('rebuild');`); e != nil {
		return recall.ErrUnavailable
	}
	if tx.Commit() != nil {
		return recall.ErrUnavailable
	}
	return s.cleanupSealing(ctx)
}
func (s *Store) cleanupSealing(ctx context.Context) error {
	if _, e := s.db.ExecContext(ctx, `VACUUM`); e != nil {
		return recall.ErrUnavailable
	}
	var busy, log, checkpointed int
	if s.db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &log, &checkpointed) != nil || busy != 0 {
		return recall.ErrUnavailable
	}
	if _, e := s.db.ExecContext(ctx, `UPDATE memory_protection SET cleanup_pending=0 WHERE singleton=1`); e != nil {
		return recall.ErrUnavailable
	}
	return nil
}
