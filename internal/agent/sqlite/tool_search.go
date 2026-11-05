package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/tool"
)

func (t *tools) searchTools(ctx context.Context, inv tool.Invocation) (tool.Result, error) {
	result := tool.Result{}
	var request tool.SearchRequest
	var specs []tool.Spec
	var cp agent.Checkpoint
	err := t.runs.write(ctx, func(tx *sql.Tx) error {
		_, name, args, err := t.acceptedBuiltin(ctx, tx, inv)
		if err != nil {
			return err
		}
		if name != tool.SearchName || json.Unmarshal([]byte(args), &request) != nil || strings.TrimSpace(request.Query) == "" {
			result = tool.Failure("INVALID_ARGUMENT")
			return nil
		}
		if request.Limit == 0 {
			request.Limit = tool.DefaultSearchLimit
		}
		if request.Limit < 1 || request.Limit > tool.MaxSearchLimit {
			result = tool.Failure("INVALID_ARGUMENT")
			return nil
		}
		var frozen, checkpoint string
		if err := tx.QueryRowContext(ctx, `SELECT c.tools_json,r.checkpoint_json FROM run r JOIN capability_snapshot c ON c.snapshot_id=r.capability_snapshot_id WHERE r.run_id=?`, t.claim.RunID()).Scan(&frozen, &checkpoint); err != nil {
			return err
		}
		if json.Unmarshal([]byte(frozen), &specs) != nil || json.Unmarshal([]byte(checkpoint), &cp) != nil {
			return agent.ErrStorage
		}
		return nil
	})
	if err != nil || result.Error != nil {
		return result, err
	}
	// Ranking is pure frozen metadata work. Do not hold SQLite's write lock
	// while examining a large catalog; new user inputs must remain admissible.
	matches := tool.SearchTools(specs, cp.LoadedTools, request.Query, request.Limit)
	text, err := encoded(matches)
	return tool.Result{Text: text}, err
}
