// Package sqlite owns read-only metadata projections over the three domain
// databases. It has no writer, migration, Vault, embedding or model dependency.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/z-chenhao/assistant/internal/database"
	_ "modernc.org/sqlite"
)

type Reader struct{ state, history, memory *sql.DB }

// Open creates lazy, independent pools. Missing databases are never created.
func Open(root string) (*Reader, error) {
	if !filepath.IsAbs(root) {
		return nil, database.ErrInvalid
	}
	r := &Reader{}
	for name, target := range map[string]**sql.DB{"state.db": &r.state, "conversation.db": &r.history, "memory.db": &r.memory} {
		u := url.URL{Scheme: "file", Path: filepath.Join(root, name)}
		q := u.Query()
		q.Set("mode", "ro")
		q.Add("_pragma", "query_only(1)")
		q.Add("_pragma", "busy_timeout(500)")
		u.RawQuery = q.Encode()
		db, err := sql.Open("sqlite", u.String())
		if err != nil {
			r.Close()
			return nil, database.ErrUnavailable
		}
		db.SetMaxOpenConns(2)
		db.SetMaxIdleConns(2)
		*target = db
	}
	return r, nil
}
func (r *Reader) Close() error {
	var err error
	for _, db := range []*sql.DB{r.state, r.history, r.memory} {
		if db != nil {
			err = errors.Join(err, db.Close())
		}
	}
	return err
}

type scanner interface{ Scan(...any) error }
type row struct {
	database.Record
	id      string
	at      int64
	sources []source
}
type source struct{ Kind, Ref string }
type projection struct {
	db           *sql.DB
	columns      string
	from         string
	id, position string
	args         []any
	scan         func(scanner) (row, error)
}

func (r *Reader) owner(ctx context.Context) (string, error) {
	var owner string
	if err := r.state.QueryRowContext(ctx, `SELECT agent_id FROM agent_config WHERE singleton=1`).Scan(&owner); err != nil || owner == "" {
		return "", database.ErrUnavailable
	}
	return owner, nil
}

func (r *Reader) Browse(ctx context.Context, q database.Query) (database.Page, error) {
	out := database.Page{Collection: q.Collection, Items: []database.Record{}}
	if err := q.Validate(); err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	owner, err := r.owner(ctx)
	if err != nil {
		return out, err
	}
	cursor, err := database.Position(q, owner)
	if err != nil {
		return out, err
	}
	p := r.projection(q.Collection, owner)
	query, args := "SELECT "+p.columns+p.from, p.args
	if q.Ref != "" {
		id := q.Ref
		if q.Collection != "artifacts" {
			_, id, _ = strings.Cut(q.Ref, ":")
		}
		query += " AND " + p.id + "=?"
		args = append(args, id)
	}
	if cursor.ID != "" {
		query += " AND (" + p.position + "," + p.id + ")<(?,?)"
		args = append(args, cursor.At, cursor.ID)
	}
	query += " ORDER BY " + p.position + " DESC," + p.id + " DESC LIMIT ?"
	args = append(args, q.PageSize()+1)
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return out, database.ErrUnavailable
	}
	var last row
	sources := map[string][]source{}
	for rows.Next() {
		if len(out.Items) == q.PageSize() {
			out.Next = database.Next(q.Collection, owner, last.at, last.id)
			break
		}
		last, err = p.scan(rows)
		if err != nil {
			rows.Close()
			return database.Page{}, database.ErrUnavailable
		}
		out.Items = append(out.Items, last.Record)
		if len(last.sources) != 0 {
			sources[last.Ref] = last.sources
		}
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return database.Page{}, database.ErrUnavailable
	}
	if q.Ref != "" && len(out.Items) == 0 {
		return out, database.ErrNotFound
	}
	if q.Collection == "messages" {
		if err = r.messageExecutions(ctx, owner, out.Items); err != nil {
			return database.Page{}, database.ErrUnavailable
		}
	}
	if q.Collection == "memories" {
		if err = r.memoryExecutions(ctx, owner, sources, out.Items); err != nil {
			return database.Page{}, database.ErrUnavailable
		}
	}
	if err = r.filterLinks(ctx, owner, out.Items); err != nil {
		return database.Page{}, database.ErrUnavailable
	}
	current, err := r.owner(ctx)
	if err != nil || current != owner {
		return database.Page{}, database.ErrUnavailable
	}
	return out, nil
}
