package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/coding"
	"github.com/z-chenhao/assistant/internal/computer/environment"
	"github.com/z-chenhao/assistant/internal/llm"
)

func (r *Runs) bindTaskCode(ctx context.Context, tx *sql.Tx, id string) error {
	var scope string
	err := tx.QueryRowContext(ctx, `SELECT scope_hash FROM task_code_environment WHERE task_id=?`, id).Scan(&scope)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		if r.localEnvironment == nil || scope != environment.ResourceHash(*r.localEnvironment) {
			return llm.ModelIncompatible
		}
		return nil
	}
	if r.localEnvironment == nil || r.localEnvironment.RepositorySource == "" {
		return nil
	}
	cfg := *r.localEnvironment
	base := filepath.Join(cfg.Root, ".tasks", digest(id)[:32])
	_, err = tx.ExecContext(ctx, `INSERT INTO task_code_environment(task_id,scope_hash,source_path,directory,baseline_path,status) VALUES(?,?,?,?,?,'pending')`, id, environment.ResourceHash(cfg), cfg.RepositorySource, filepath.Join(base, "work"), filepath.Join(base, "baseline"))
	return err
}

func (r *Runs) prepareTaskCode(ctx context.Context, c agent.RunClaim, run agent.Run) error {
	var id, scope, source, status string
	var existing coding.Workspace
	err := r.host.db.QueryRowContext(ctx, `SELECT e.task_id,e.scope_hash,e.source_path,e.status,e.directory,e.baseline_path,e.baseline_hash FROM task_code_environment e JOIN run r USING(task_id) WHERE r.run_id=?`, c.RunID()).Scan(&id, &scope, &source, &status, &existing.Directory, &existing.Baseline, &existing.BaselineHash)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return agent.ErrStorage
	}
	if r.localEnvironment == nil || scope != environment.ResourceHash(*r.localEnvironment) {
		return llm.ModelIncompatible
	}
	// Do not bind a prior frozen Run to a newly configured source. Registry
	// selection would reject it too, but initialization must remain read-only.
	for _, spec := range run.Tools {
		if environment.IsTool(spec.Name) && spec.ResourceHash != scope {
			return llm.ModelIncompatible
		}
	}
	if status == "ready" {
		if coding.Validate(ctx, existing) == nil {
			return nil
		}
		if err := r.Fail(ctx, c, run.Version, string(llm.ProviderUnavailable)); err != nil {
			return err
		}
		return agent.CommittedRunFailure(llm.ProviderUnavailable)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if r.write(ctx, func(tx *sql.Tx) error { return fence(ctx, tx, c) }) != nil {
					cancel()
					return
				}
			}
		}
	}()
	w, prepareErr := coding.Prepare(ctx, r.localEnvironment.Root, source, id, scope)
	cancel()
	<-done
	state, code := "ready", ""
	if prepareErr != nil {
		state, code = "failed", "CODE_PREPARATION_FAILED"
	}
	err = r.write(context.WithoutCancel(ctx), func(tx *sql.Tx) error {
		if err := fence(context.WithoutCancel(ctx), tx, c); err != nil {
			return err
		}
		_, err := tx.ExecContext(context.WithoutCancel(ctx), `UPDATE task_code_environment SET status=?,baseline_hash=?,error_code=? WHERE task_id=? AND status!='ready'`, state, w.BaselineHash, code, id)
		return err
	})
	if err != nil {
		return err
	}
	if prepareErr != nil {
		if err := r.Fail(context.WithoutCancel(ctx), c, run.Version, string(llm.ProviderUnavailable)); err != nil {
			return err
		}
		return agent.CommittedRunFailure(llm.ProviderUnavailable)
	}
	return nil
}

func (s *Store) CodeReport(ctx context.Context, ref string, withDiff bool) (coding.Report, error) {
	var out coding.Report
	var taskID string
	err := s.db.QueryRowContext(ctx, `SELECT t.task_id,t.task_ref,e.directory,e.baseline_path,e.baseline_hash,e.status,e.error_code FROM task_code_environment e JOIN task t USING(task_id) WHERE t.task_ref=? AND t.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1)`, ref).Scan(&taskID, &out.TaskRef, &out.Directory, &out.Baseline, &out.BaselineHash, &out.Status, &out.ErrorCode)
	if err == sql.ErrNoRows {
		return out, agent.ErrNotFound
	}
	if err != nil {
		return out, agent.ErrStorage
	}
	out.Commands = []coding.Command{}
	rows, err := s.db.QueryContext(ctx, `SELECT i.arguments_json,i.status,coalesce(i.result_json,'{}') FROM tool_invocation i JOIN run r USING(run_id) WHERE r.task_id=? AND i.tool_name='builtin__bash' ORDER BY i.created_at,i.invocation_id`, taskID)
	if err != nil {
		return out, agent.ErrStorage
	}
	for rows.Next() {
		var args, raw string
		var command coding.Command
		if err = rows.Scan(&args, &command.Status, &raw); err != nil {
			rows.Close()
			return out, agent.ErrStorage
		}
		var input struct {
			Command string `json:"command"`
		}
		if json.Unmarshal([]byte(args), &input) != nil {
			rows.Close()
			return out, agent.ErrStorage
		}
		command.Command = input.Command
		command.Result = json.RawMessage(raw)
		out.Commands = append(out.Commands, command)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, agent.ErrStorage
	}
	if withDiff {
		if out.Status != "ready" {
			return out, coding.ErrInvalid
		}
		diff, err := coding.Review(ctx, out.Workspace)
		if err != nil {
			return out, err
		}
		out.Diff = &diff
	}
	return out, nil
}
