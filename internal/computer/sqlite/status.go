package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/z-chenhao/collie/internal/computer"
)

func Status(ctx context.Context, db *sql.DB) (computer.Status, error) {
	var status computer.Status
	var raw sql.NullString
	err := db.QueryRowContext(ctx, `SELECT computer_id,instance_epoch,phase,checkpoint_json FROM computer WHERE singleton=1`).Scan(&status.ComputerID, &status.Epoch, &status.Phase, &raw)
	if err == sql.ErrNoRows {
		status.Phase = "unprovisioned"
		return status, nil
	}
	if err != nil {
		return computer.Status{}, ErrStorage
	}
	if raw.Valid && json.Unmarshal([]byte(raw.String), &status.Checkpoint) != nil {
		return computer.Status{}, computer.ErrIntegrity
	}
	return status, nil
}
