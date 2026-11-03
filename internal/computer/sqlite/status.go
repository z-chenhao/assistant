package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/z-chenhao/assistant/internal/computer"
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

// PreviewInstance never provisions or resumes a Computer. No transaction is held
// while the caller captures a frame; callers revalidate the epoch afterward.
func PreviewInstance(ctx context.Context, db *sql.DB) (computer.Instance, error) {
	var raw string
	var instance computer.Instance
	err := db.QueryRowContext(ctx, `SELECT c.instance_json FROM computer c
 JOIN tool_invocation i ON i.invocation_id=c.execution_id JOIN run r ON r.run_id=i.run_id
 WHERE c.singleton=1 AND c.phase='ready' AND r.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1)`).Scan(&raw)
	if err == sql.ErrNoRows {
		return instance, computer.ErrStale
	}
	if err != nil {
		return instance, ErrStorage
	}
	if json.Unmarshal([]byte(raw), &instance) != nil || instance.ComputerID == "" || instance.Epoch < 1 || instance.NativeName == "" {
		return computer.Instance{}, computer.ErrIntegrity
	}
	return instance, nil
}
