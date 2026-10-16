package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/llm"
	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func Open(ctx context.Context, root string) (*Store, error) {
	if !filepath.IsAbs(root) {
		return nil, conversation.ErrStorage
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, conversation.ErrStorage
	}
	path := filepath.Join(root, "conversation.db")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, conversation.ErrStorage
	}
	if f.Close() != nil {
		return nil, conversation.ErrStorage
	}
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	for _, p := range []string{"foreign_keys(1)", "busy_timeout(5000)", "journal_mode(WAL)"} {
		q.Add("_pragma", p)
	}
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, conversation.ErrStorage
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return conversation.ErrStorage
	}
	defer tx.Rollback()
	var version int
	if tx.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version) != nil || version < 0 || version > 1 {
		return conversation.ErrStorage
	}
	if version == 0 {
		_, err = tx.ExecContext(ctx, `CREATE TABLE conversation_message (
message_id TEXT PRIMARY KEY, conversation_id TEXT NOT NULL, sequence INTEGER NOT NULL,
role TEXT NOT NULL CHECK(role IN ('user','assistant')), content_json TEXT NOT NULL CHECK(json_valid(content_json)),
source TEXT NOT NULL, source_event_id TEXT, created_at INTEGER NOT NULL,
UNIQUE(conversation_id,sequence)) STRICT;
CREATE TRIGGER conversation_no_update BEFORE UPDATE ON conversation_message BEGIN SELECT RAISE(ABORT,'immutable message'); END;
CREATE TRIGGER conversation_no_delete BEFORE DELETE ON conversation_message BEGIN SELECT RAISE(ABORT,'immutable message'); END;
CREATE TRIGGER conversation_no_replace BEFORE INSERT ON conversation_message WHEN EXISTS(SELECT 1 FROM conversation_message WHERE message_id=NEW.message_id) BEGIN SELECT RAISE(ABORT,'immutable message'); END;
PRAGMA user_version=1;`)
		if err != nil {
			return conversation.ErrStorage
		}
	}
	if tx.Commit() != nil {
		return conversation.ErrStorage
	}
	return nil
}

// Append atomically assigns monotonic sequences in supplied intent order.
// Repeating a previously committed prefix is harmless; changed IDs fail closed.
func (s *Store) Append(ctx context.Context, messages []conversation.Message) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return conversation.ErrStorage
	}
	defer tx.Rollback()
	for _, m := range messages {
		parts, err := conversation.Normalize(m.Content)
		if err != nil || m.MessageID == "" || m.ConversationID == "" || (m.Role != "user" && m.Role != "assistant") || m.Source == "" || m.CreatedAt.IsZero() {
			return conversation.ErrInvalid
		}
		for _, p := range parts {
			if p.Type == "tool_call" {
				return conversation.ErrInvalid
			}
		}
		data, err := llm.CanonicalJSON(parts)
		if err != nil {
			return conversation.ErrInvalid
		}
		var c, role, content, source, event string
		var created int64
		err = tx.QueryRowContext(ctx, `SELECT conversation_id,role,content_json,source,coalesce(source_event_id,''),created_at FROM conversation_message WHERE message_id=?`, m.MessageID).Scan(&c, &role, &content, &source, &event, &created)
		if err == nil {
			if c != m.ConversationID || role != m.Role || content != string(data) || source != m.Source || event != m.SourceEventID || created != m.CreatedAt.UnixMicro() {
				return conversation.ErrConflict
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return conversation.ErrStorage
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO conversation_message VALUES(?,?,(SELECT coalesce(max(sequence),0)+1 FROM conversation_message WHERE conversation_id=?),?,?,?,?,?)`, m.MessageID, m.ConversationID, m.ConversationID, m.Role, string(data), m.Source, m.SourceEventID, m.CreatedAt.UnixMicro())
		if err != nil {
			return conversation.ErrStorage
		}
	}
	if tx.Commit() != nil {
		return conversation.ErrStorage
	}
	return nil
}

func (s *Store) List(ctx context.Context, id string) ([]conversation.Message, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT message_id,sequence,role,content_json,source,coalesce(source_event_id,''),created_at FROM conversation_message WHERE conversation_id=? ORDER BY sequence`, id)
	if err != nil {
		return nil, conversation.ErrStorage
	}
	defer rows.Close()
	result := []conversation.Message{}
	for rows.Next() {
		var m conversation.Message
		var raw string
		var created int64
		m.ConversationID = id
		if rows.Scan(&m.MessageID, &m.Sequence, &m.Role, &raw, &m.Source, &m.SourceEventID, &created) != nil || json.Unmarshal([]byte(raw), &m.Content) != nil {
			return nil, conversation.ErrStorage
		}
		m.CreatedAt = time.UnixMicro(created).UTC()
		result = append(result, m)
	}
	if rows.Err() != nil {
		return nil, conversation.ErrStorage
	}
	return result, nil
}
func (s *Store) Close() error { return s.db.Close() }
