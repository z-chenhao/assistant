package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/memory"
	"github.com/z-chenhao/erii/internal/recall"
	"github.com/z-chenhao/erii/internal/recall/sqliteindex"
	_ "modernc.org/sqlite"
)

type Store struct {
	db     *sql.DB
	engine recall.Embedding
}

const schema = `CREATE TABLE memory_record(
 memory_id TEXT PRIMARY KEY,user_id TEXT NOT NULL,scope_type TEXT NOT NULL CHECK(scope_type IN('user','agent')),
 scope_id TEXT NOT NULL,tier TEXT NOT NULL CHECK(tier IN('core','recall')),content TEXT NOT NULL,
 source_refs TEXT NOT NULL CHECK(json_valid(source_refs)),supersedes_memory_id TEXT REFERENCES memory_record(memory_id),
 created_at INTEGER NOT NULL,deleted_at INTEGER,version INTEGER NOT NULL DEFAULT 1 CHECK(version>=1)
) STRICT;
CREATE UNIQUE INDEX memory_one_successor ON memory_record(supersedes_memory_id) WHERE supersedes_memory_id IS NOT NULL;
CREATE TRIGGER memory_immutable BEFORE UPDATE OF memory_id,user_id,scope_type,scope_id,tier,content,source_refs,supersedes_memory_id,created_at ON memory_record BEGIN SELECT RAISE(ABORT,'immutable memory'); END;
CREATE TRIGGER memory_no_delete BEFORE DELETE ON memory_record BEGIN SELECT RAISE(ABORT,'retain memory audit'); END;
CREATE TRIGGER memory_no_replace BEFORE INSERT ON memory_record WHEN EXISTS(SELECT 1 FROM memory_record WHERE memory_id=new.memory_id) BEGIN SELECT RAISE(ABORT,'immutable memory'); END;`

func Open(ctx context.Context, root string) (*Store, error) {
	if !filepath.IsAbs(root) {
		return nil, recall.ErrInvalid
	}
	if os.MkdirAll(root, 0700) != nil {
		return nil, recall.ErrUnavailable
	}
	path := filepath.Join(root, "memory.db")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, recall.ErrUnavailable
	}
	if f.Close() != nil {
		return nil, recall.ErrUnavailable
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
		return nil, recall.ErrUnavailable
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err = s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return recall.ErrUnavailable
	}
	defer tx.Rollback()
	var version int
	if tx.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version) != nil || version < 0 || version > 1 {
		return recall.ErrUnavailable
	}
	if version == 0 {
		if _, err = tx.ExecContext(ctx, schema+sqliteindex.Schema+`PRAGMA user_version=1;`); err != nil {
			return recall.ErrUnavailable
		}
	}
	if tx.Commit() != nil {
		return recall.ErrUnavailable
	}
	return nil
}
func (s *Store) Configure(ctx context.Context, e recall.Embedding) error {
	if err := sqliteindex.Bind(ctx, s.db, e); err != nil {
		return err
	}
	s.engine = e
	return nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Search(ctx context.Context, scope recall.Scope, q recall.Query) ([]recall.Hit, error) {
	return sqliteindex.Search(ctx, s.db, scope, q, s.engine, "memory")
}
func (s *Store) Apply(ctx context.Context, r memory.Record) error {
	if r.ID == "" || r.UserID == "" || r.ScopeID == "" || (r.ScopeType != "user" && r.ScopeType != "agent") || (r.ScopeType == "user" && r.ScopeID != r.UserID) || (r.Tier != "core" && r.Tier != "recall") || strings.TrimSpace(r.Content) == "" || len(r.Content) > 64<<10 || r.CreatedAt.IsZero() || len(r.SourceRefs) == 0 {
		return recall.ErrInvalid
	}
	for _, ref := range r.SourceRefs {
		if ref.Ref == "" || (ref.Type != "conversation" && ref.Type != "task" && ref.Type != "run" && ref.Type != "feedback") {
			return recall.ErrInvalid
		}
	}
	refs, _ := llm.CanonicalJSON(r.SourceRefs)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return recall.ErrUnavailable
	}
	defer tx.Rollback()
	var old memory.Record
	var raw string
	var created int64
	err = tx.QueryRowContext(ctx, `SELECT memory_id,user_id,scope_type,scope_id,tier,content,source_refs,coalesce(supersedes_memory_id,''),created_at FROM memory_record WHERE memory_id=?`, r.ID).Scan(&old.ID, &old.UserID, &old.ScopeType, &old.ScopeID, &old.Tier, &old.Content, &raw, &old.Supersedes, &created)
	if err == nil {
		if json.Unmarshal([]byte(raw), &old.SourceRefs) != nil {
			return recall.ErrUnavailable
		}
		old.CreatedAt = time.UnixMicro(created).UTC()
		r.CreatedAt = r.CreatedAt.UTC().Truncate(time.Microsecond)
		a, _ := llm.CanonicalJSON(old)
		b, _ := llm.CanonicalJSON(r)
		if string(a) != string(b) {
			return recall.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return recall.ErrUnavailable
	}
	if r.Supersedes != "" {
		var valid int
		if tx.QueryRowContext(ctx, `SELECT count(*) FROM memory_record m JOIN recall_document d ON d.ref='memory:'||m.memory_id WHERE m.memory_id=? AND m.user_id=? AND m.scope_type=? AND m.scope_id=? AND m.deleted_at IS NULL AND d.active=1`, r.Supersedes, r.UserID, r.ScopeType, r.ScopeID).Scan(&valid) != nil {
			return recall.ErrUnavailable
		}
		if valid != 1 {
			return recall.ErrConflict
		}
	}
	var supersedes any
	if r.Supersedes != "" {
		supersedes = r.Supersedes
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO memory_record(memory_id,user_id,scope_type,scope_id,tier,content,source_refs,supersedes_memory_id,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, r.ID, r.UserID, r.ScopeType, r.ScopeID, r.Tier, r.Content, string(refs), supersedes, r.CreatedAt.UnixMicro()); err != nil {
		return recall.ErrUnavailable
	}
	agentID := ""
	if r.ScopeType == "agent" {
		agentID = r.ScopeID
	}
	if err = sqliteindex.Insert(ctx, tx, sqliteindex.Document{Ref: "memory:" + r.ID, UserID: r.UserID, AgentID: agentID, Content: r.Content, OccurredAt: r.CreatedAt.UnixMicro(), Core: r.Tier == "core", Active: true}); err != nil {
		return recall.ErrUnavailable
	}
	if r.Supersedes != "" {
		if _, err = tx.ExecContext(ctx, `UPDATE recall_document SET active=0 WHERE ref=?`, "memory:"+r.Supersedes); err != nil {
			return recall.ErrUnavailable
		}
	}
	if tx.Commit() != nil {
		return recall.ErrUnavailable
	}
	return nil
}
func (s *Store) Forget(ctx context.Context, scope recall.Scope, id string, at time.Time) error {
	if scope.UserID == "" || scope.AgentID == "" || at.IsZero() {
		return recall.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return recall.ErrUnavailable
	}
	defer tx.Rollback()
	var version int64
	var deleted sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT version,deleted_at FROM memory_record WHERE memory_id=? AND user_id=? AND ((scope_type='user' AND scope_id=?) OR (scope_type='agent' AND scope_id=?))`, id, scope.UserID, scope.UserID, scope.AgentID).Scan(&version, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return recall.ErrConflict
	}
	if err != nil {
		return recall.ErrUnavailable
	}
	if !deleted.Valid {
		res, err := tx.ExecContext(ctx, `UPDATE memory_record SET deleted_at=?,version=version+1 WHERE memory_id=? AND version=? AND deleted_at IS NULL`, at.UnixMicro(), id, version)
		if err != nil {
			return recall.ErrUnavailable
		}
		n, err := res.RowsAffected()
		if err != nil || n != 1 {
			return recall.ErrConflict
		}
	}
	// Forget the whole supersession lineage: older derived facts cannot reappear.
	if _, err = tx.ExecContext(ctx, `UPDATE recall_document SET active=0 WHERE ref IN(WITH RECURSIVE lineage(id) AS (SELECT ? UNION SELECT m.memory_id FROM memory_record m JOIN lineage l ON m.supersedes_memory_id=l.id UNION SELECT m.supersedes_memory_id FROM memory_record m JOIN lineage l ON m.memory_id=l.id WHERE m.supersedes_memory_id IS NOT NULL) SELECT 'memory:'||id FROM lineage)`, id); err != nil {
		return recall.ErrUnavailable
	}
	if tx.Commit() != nil {
		return recall.ErrUnavailable
	}
	return nil
}
