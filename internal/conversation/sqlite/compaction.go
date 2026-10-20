package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/z-chenhao/erii/internal/conversation"
	"time"
)

const compactionSchema = `CREATE TABLE compaction_summary(ref TEXT PRIMARY KEY,conversation_id TEXT NOT NULL,summary_json TEXT NOT NULL CHECK(json_valid(summary_json)),provenance_json TEXT NOT NULL CHECK(json_valid(provenance_json)),created_at INTEGER NOT NULL) STRICT;
CREATE TRIGGER summary_immutable BEFORE UPDATE ON compaction_summary BEGIN SELECT RAISE(ABORT,'immutable summary'); END;
CREATE TRIGGER summary_audit BEFORE DELETE ON compaction_summary BEGIN SELECT RAISE(ABORT,'retain summary provenance'); END;
CREATE TRIGGER summary_no_replace BEFORE INSERT ON compaction_summary WHEN EXISTS(SELECT 1 FROM compaction_summary WHERE ref=NEW.ref) BEGIN SELECT RAISE(ABORT,'summary exists'); END;`

// Summaries are derived context, never semantic Conversation messages or new
// retrieval evidence. Originals and runtime origins remain authoritative.
func (s *Store) SaveCompaction(ctx context.Context, ref, conversationID, summary, provenance string) error {
	if ref == "" || conversationID == "" || !json.Valid([]byte(summary)) || !json.Valid([]byte(provenance)) {
		return conversation.ErrInvalid
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return conversation.ErrStorage
	}
	defer tx.Rollback()
	var owner, old, refs string
	e = tx.QueryRowContext(ctx, `SELECT conversation_id,summary_json,provenance_json FROM compaction_summary WHERE ref=?`, ref).Scan(&owner, &old, &refs)
	if e == nil {
		if owner != conversationID || old != summary || refs != provenance {
			return conversation.ErrConflict
		}
		return nil
	}
	if e != sql.ErrNoRows {
		return conversation.ErrStorage
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO compaction_summary VALUES(?,?,?,?,?)`, ref, conversationID, summary, provenance, time.Now().UnixMicro()); e != nil {
		return conversation.ErrStorage
	}
	if tx.Commit() != nil {
		return conversation.ErrStorage
	}
	return nil
}
