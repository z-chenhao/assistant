package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// A local pre-release build used version 47 for semantic Memory before main
// allocated 47 to cost estimates. Only that exact, complete private schema is
// adopted: its sealed journals remain in place and the missing public migration
// runs instead. Public v47 follows the ordinary v48 migration. Never relax DDL
// globally or lower user_version to replay already accepted work.
func adoptSemantic47(ctx context.Context, tx *sql.Tx, version, index int, scripts []string) (string, error) {
	script := scripts[index]
	if version != 47 || index != 47 || scripts[46] != costEstimatesSQL || script != semanticMemorySQL {
		return script, nil
	}
	var costs int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE type='table' AND name='generation_cost_estimate'`).Scan(&costs); err != nil {
		return "", err
	}
	if costs != 0 {
		return script, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT sql FROM sqlite_schema WHERE name IN ('memory_generation','memory_generation_event','memory_semantic_receipt','memory_cost_reservation')`)
	if err != nil {
		return "", err
	}
	count := 0
	for rows.Next() {
		var definition string
		if err = rows.Scan(&definition); err != nil {
			break
		}
		count++
		if !strings.Contains(semanticMemorySQL, strings.TrimSpace(definition)+";") {
			err = errors.New("unrecognized pre-release memory schema")
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return "", err
	}
	if count != 4 {
		return "", errors.New("incomplete pre-release memory schema")
	}
	return costEstimatesSQL, nil
}
