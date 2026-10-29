package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"unicode/utf8"

	"github.com/z-chenhao/erii/internal/observability"
)

func contentChunk(body []byte, offset, length int) (string, *int, error) {
	if offset > length {
		return "", nil, observability.ErrNotFound
	}
	if len(body) > 0 && !utf8.RuneStart(body[0]) {
		return "", nil, observability.ErrInvalid
	}
	if offset+len(body) < length {
		original := len(body)
		for len(body) > 0 && !utf8.Valid(body) {
			if original-len(body) == utf8.UTFMax-1 {
				return "", nil, errors.New("invalid frozen text")
			}
			body = body[:len(body)-1]
		}
	}
	if !utf8.Valid(body) {
		return "", nil, errors.New("invalid frozen text")
	}
	if offset+len(body) < length {
		next := offset + len(body)
		return string(body), &next, nil
	}
	return string(body), nil, nil
}

// The SELECT is a fixed, scoped projection chosen by the adapter. All bodies
// share byte/NUL/UTF-8 handling; storage format never leaks into Web handlers.
func readBody(ctx context.Context, tx *sql.Tx, selection string, args []any, offset int) (observability.Body, error) {
	out := observability.Body{}
	query := `WITH payload(body) AS (` + selection + `) SELECT body IS NOT NULL,substr(cast(body AS blob),?,?),coalesce(length(cast(body AS blob)),0) FROM payload`
	args = append(args, offset+1, observability.ContextChunk)
	var raw []byte
	err := tx.QueryRowContext(ctx, query, args...).Scan(&out.Available, &raw, &out.Length)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Content, out.Next, err = contentChunk(raw, offset, out.Length)
	return out, err
}
