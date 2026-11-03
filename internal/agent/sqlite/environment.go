package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"time"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/computer/environment"
	"github.com/z-chenhao/assistant/internal/tool"
	"github.com/z-chenhao/assistant/internal/tool/codemode"
)

func (r *Runs) ConfigureEnvironment(execute func(context.Context, string, string) tool.Result) {
	r.environmentExecute = execute
}

func (r *Runs) ConfigureLocalEnvironment(cfg environment.Config) {
	r.localEnvironment = &cfg
	r.environmentExecute = (environment.Local{Config: cfg}).Execute
	r.localExecute = func(ctx context.Context, configuration environment.Config, name, args string) tool.Result {
		return (environment.Local{Config: configuration}).Execute(ctx, name, args)
	}
}

func (t *tools) executeEnvironment(ctx context.Context, inv tool.Invocation) (result tool.Result, executionErr error) {
	if err := t.runs.write(ctx, func(tx *sql.Tx) error {
		_, name, arguments, err := t.acceptedBuiltin(ctx, tx, inv)
		if err == nil {
			inv.Call.Name, inv.Call.Arguments = name, arguments
		}
		return err
	}); err != nil {
		return tool.Result{}, err
	}
	driver, ok := t.runs.computerProvider.(environment.GuestDriver)
	if t.runs.computerProvider != nil && !ok {
		return tool.Failure("PROVIDER_UNAVAILABLE"), nil
	}
	b := t.codeOwner
	if b == nil && t.runs.computerProvider != nil {
		// Reuse the existing durable execution claim/lifecycle ledger for native
		// code and file calls too. There is no generated JavaScript, nested Tool
		// call, second approval, or second invocation lifecycle.
		b = &codeBridge{t: t, req: codemode.ExecutionRequest{ExecutionID: inv.ID, ParentInvocationID: inv.ID, RunID: t.claim.RunID(), Source: inv.Call.Name + ":" + inv.Call.Arguments, SourceHash: digest(inv.Call.Name + ":" + inv.Call.Arguments), RuntimeVersion: "steward-native-v1", PreludeVersion: "none"}}
		if err := b.claim(ctx, inv); err != nil {
			return tool.Result{}, err
		}
		defer func() {
			result.HostFence = codeClaimFence{b.req.ClaimToken, b.req.ExecutionEpoch}
			if executionErr != nil {
				executionErr = errors.Join(executionErr, b.abandon())
			}
		}()
	}
	// Stop transports when either the Run claim or a containing Code claim is
	// lost. Cancellation interrupts the adapter; possible effects stay unknown.
	live, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-live.Done():
				return
			case <-tick.C:
				if t.runs.write(live, func(tx *sql.Tx) error {
					if b != nil {
						return b.fence(live, tx)
					}
					return t.fence(live, tx)
				}) != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-done }()
	if t.runs.computerProvider == nil {
		if t.runs.localEnvironment != nil {
			cfg := *t.runs.localEnvironment
			var taskID string
			if err := t.runs.host.db.QueryRowContext(live, `SELECT coalesce(task_id,'') FROM run WHERE run_id=?`, t.claim.RunID()).Scan(&taskID); err != nil {
				return tool.Result{}, agent.ErrStorage
			}
			if taskID != "" {
				var directory, scope, status string
				err := t.runs.host.db.QueryRowContext(live, `SELECT directory,scope_hash,status FROM task_code_environment WHERE task_id=?`, taskID).Scan(&directory, &scope, &status)
				if err != nil && err != sql.ErrNoRows {
					return tool.Result{}, agent.ErrStorage
				}
				if err == nil {
					if scope != environment.ResourceHash(cfg) || status != "ready" {
						return tool.Failure("PERMISSION_DENIED"), nil
					}
					info, err := os.Lstat(directory)
					if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
						return tool.Failure("PROVIDER_UNAVAILABLE"), nil
					}
					cfg.Root = directory
					return t.runs.localExecute(live, cfg, inv.Call.Name, inv.Call.Arguments), nil
				}
			}
		}
		if t.runs.environmentExecute == nil {
			return tool.Failure("PROVIDER_UNAVAILABLE"), nil
		}
		return t.runs.environmentExecute(live, inv.Call.Name, inv.Call.Arguments), nil
	}
	instance, err := b.computer().Ensure(live)
	if err != nil {
		if errors.Is(err, agent.ErrClaimLost) || errors.Is(err, agent.ErrStorage) {
			return tool.Result{}, err
		}
		return computerFailure(err), nil
	}
	result = driver.ExecuteEnvironment(live, instance, inv.Call.Name, inv.Call.Arguments)
	if t.codeOwner == nil && live.Err() == nil {
		// Nested calls leave disposal to their cell, after all siblings drain.
		// A direct call checkpoints before delivery. Unprovable mutations remain
		// unknown; recovery never repeats the accepted effect.
		if err := b.computer().Idle(live); err != nil {
			if errors.Is(err, agent.ErrClaimLost) || errors.Is(err, agent.ErrStorage) {
				return tool.Result{}, err
			}
			if inv.Call.Name != "builtin__read" {
				return tool.Failure("UNKNOWN_SIDE_EFFECT"), nil
			}
			return computerFailure(err), nil
		}
	}
	return result, nil
}
