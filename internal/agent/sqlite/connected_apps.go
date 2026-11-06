package sqlite

import (
	"context"
	"encoding/json"

	"github.com/z-chenhao/assistant/internal/connectedapps"
)

func (s *Store) LoadApps(ctx context.Context) (connectedapps.State, error) {
	var state connectedapps.State
	var document string
	err := s.db.QueryRowContext(ctx, `SELECT revision,document_json FROM connected_apps WHERE singleton=1`).Scan(&state.Revision, &document)
	if err != nil {
		return state, err
	}
	err = json.Unmarshal([]byte(document), &state)
	return state, err
}
func (s *Store) SaveApps(ctx context.Context, state connectedapps.State, revision int64) error {
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE connected_apps SET revision=revision+1,document_json=? WHERE singleton=1 AND revision=?`, string(b), revision)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return connectedapps.ErrConflict
	}
	return nil
}
