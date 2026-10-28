package sqlite

import (
	"context"
	"database/sql"
	"github.com/z-chenhao/erii/internal/recall"
	"github.com/z-chenhao/erii/internal/recall/sqliteindex"
)

func (s *Store) lockSearch(ctx context.Context) error {
	select {
	case s.searchGate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Store) unlockSearch() { <-s.searchGate }
func (s *Store) clearWorking() {
	if s.working != nil {
		_ = s.working.Close()
	}
	s.working = nil
	s.indexed = nil
	s.workingScope = recall.Scope{}
}

// The encrypted corpus has one scoped, private in-memory search index per
// Store. Switching scopes discards it. Reconciliation preserves unchanged
// document vectors, removes superseded/forgotten records, and indexes only new
// facts. Neither plaintext nor vectors are persisted in memory.db or temp files.
func (s *Store) syncWorking(ctx context.Context, scope recall.Scope, docs []sqliteindex.Document) error {
	if s.workingScope != scope {
		s.clearWorking()
	}
	if s.working == nil {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			return recall.ErrUnavailable
		}
		db.SetMaxOpenConns(1)
		if _, err = db.ExecContext(ctx, `PRAGMA temp_store=MEMORY; PRAGMA journal_mode=MEMORY;`+sqliteindex.Schema); err != nil {
			db.Close()
			return recall.ErrUnavailable
		}
		s.working, s.workingScope, s.indexed = db, scope, make(map[string]sqliteindex.Document)
	}
	next := make(map[string]sqliteindex.Document, len(docs))
	for _, d := range docs {
		next[d.Ref] = d
	}
	tx, err := s.working.BeginTx(ctx, nil)
	if err != nil {
		return recall.ErrUnavailable
	}
	defer tx.Rollback()
	for ref, old := range s.indexed {
		if current, ok := next[ref]; !ok || current != old {
			if _, err = tx.ExecContext(ctx, `DELETE FROM recall_document WHERE ref=?`, ref); err != nil {
				return recall.ErrUnavailable
			}
		}
	}
	for _, d := range docs {
		if old, ok := s.indexed[d.Ref]; !ok || old != d {
			if err = sqliteindex.Insert(ctx, tx, d); err != nil {
				return recall.ErrUnavailable
			}
		}
	}
	if tx.Commit() != nil {
		return recall.ErrUnavailable
	}
	s.indexed = next
	return nil
}
