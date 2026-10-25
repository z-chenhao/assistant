package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/task"
)

func assertCompleteRejected(t *testing.T, s *Store, r *Runs, a agent.Actor, key string) {
	t.Helper()
	version := scalar(t, s, `SELECT version FROM task`)
	requests := scalar(t, s, `SELECT count(*) FROM task_request`)
	updates := scalar(t, s, `SELECT count(*) FROM task_update`)
	controlMain(t, r, a, key, "complete")
	if scalar(t, s, `SELECT version FROM task`) != version || scalar(t, s, `SELECT count(*) FROM task_request`) != requests || scalar(t, s, `SELECT count(*) FROM task_update`) != updates {
		t.Fatal("rejected completion mutated Task/receipt/update")
	}
	if scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE tool_name=? AND json_extract(arguments_json,'$.operation')='complete' AND json_extract(result_json,'$.error.code')='INVALID_ARGUMENT'`, task.ManageName) == 0 {
		t.Fatal("missing non-retryable Tool rejection")
	}
}

func TestTaskCompleteRacesAppend(t *testing.T) {
	for range 10 {
		s, r, a := controlPolicyFixture(t, nil, "ongoing")
		controlMain(t, r, a, "create", "create")
		mustDrain(t, a)
		controlMain(t, r, a, "race", "complete-race")
		if scalar(t, s, `SELECT count(*) FROM task WHERE status='active' AND task_epoch=2 AND completion_policy='ongoing'`) != 1 || scalar(t, s, `SELECT count(*) FROM task_instruction`) != 1 || scalar(t, s, `SELECT count(*) FROM run WHERE kind='worker' AND status IN ('queued','running','waiting')`) != 1 {
			t.Fatal("complete/append race lost work or left an invalid state")
		}
		mustDrain(t, a)
		if scalar(t, s, `SELECT count(*) FROM task WHERE status='active' AND task_epoch=2`) != 1 || scalar(t, s, `SELECT count(*) FROM task_update`) != 2 {
			t.Fatal("new epoch lost result")
		}
	}
}

func TestTaskCompleteIdlePoliciesDedupAndAppend(t *testing.T) {
	for _, policy := range []string{"once", "ongoing"} {
		t.Run(policy, func(t *testing.T) {
			s, r, a := controlPolicyFixture(t, func(llm.GenerationRequest) (llm.GenerationResult, error) {
				// A failed once Root leaves its Task active, eligible for explicit
				// completion. Normal ongoing success also leaves an idle active Task.
				if policy == "once" {
					return llm.GenerationResult{}, llm.ModelNotFound
				}
				return llm.GenerationResult{Text: "saved checklist"}, nil
			}, policy)
			controlMain(t, r, a, "create", "create")
			c := rootClaim(t, s, r)
			err := executeTaskRun(t, a, r, context.Background(), c)
			if policy == "once" {
				if !errors.Is(err, llm.ModelNotFound) {
					t.Fatal("wrong Root failure", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			mustDrain(t, a)
			version := scalar(t, s, `SELECT version FROM task`)
			updates := scalar(t, s, `SELECT count(*) FROM task_update`)
			mains := scalar(t, s, `SELECT count(*) FROM run WHERE kind='main'`)
			controlMain(t, r, a, "complete", "complete duplicate")
			if scalar(t, s, `SELECT count(*) FROM task WHERE status='completed' AND completed_at IS NOT NULL AND task_epoch=1 AND completion_policy=? AND version=?`, policy, version+1) != 1 {
				t.Fatal("completion lost status/timestamp/policy/version/epoch")
			}
			if scalar(t, s, `SELECT count(*) FROM task_invocation i JOIN tool_invocation v USING(invocation_id) WHERE json_extract(v.arguments_json,'$.operation')='complete'`) != 2 || scalar(t, s, `SELECT count(DISTINCT receipt_json) FROM task_invocation i JOIN tool_invocation v USING(invocation_id) WHERE json_extract(v.arguments_json,'$.operation')='complete'`) != 1 {
				t.Fatal("duplicate source did not reuse receipt")
			}
			mustDrain(t, a)
			if scalar(t, s, `SELECT count(*) FROM task_update`) != updates || scalar(t, s, `SELECT count(*) FROM main_inbox`) != updates || scalar(t, s, `SELECT count(*) FROM run WHERE kind='main'`) != mains+1 {
				t.Fatal("complete emitted synthetic Worker update/follow-up")
			}
			assertCompleteRejected(t, s, r, a, "new-source-complete")
			controlMain(t, r, a, "append", "append")
			if scalar(t, s, `SELECT count(*) FROM task WHERE status='active' AND task_epoch=2 AND completed_at IS NULL AND completion_policy=?`, policy) != 1 || scalar(t, s, `SELECT count(*) FROM run WHERE kind='worker'`) != 2 {
				t.Fatal("append did not create a new Root/epoch with the same policy")
			}
			if err := r.Progress(context.Background(), c, "late old result"); !errors.Is(err, agent.ErrClaimLost) {
				t.Fatal("old Root write accepted", err)
			}
			mustDrain(t, a)
		})
	}
}

func TestTaskCompleteRejectsNonActiveAndLiveTrees(t *testing.T) {
	for _, state := range []string{"paused", "cancelled", "queued", "running", "waiting", "child_queued", "child_waiting"} {
		t.Run(state, func(t *testing.T) {
			s, r, a := controlPolicyFixture(t, nil, "ongoing")
			ctx := context.Background()
			controlMain(t, r, a, "create", "create")
			if state == "paused" || state == "cancelled" {
				op := "pause"
				if state == "cancelled" {
					op = "cancel"
				}
				controlMain(t, r, a, op, op)
			} else if state != "queued" {
				c := rootClaim(t, s, r)
				if state == "waiting" {
					run, err := r.Load(ctx, c)
					if err != nil {
						t.Fatal(err)
					}
					if err = r.Wait(ctx, c, run.Version, "external_event"); err != nil {
						t.Fatal(err)
					}
				} else if state == "child_queued" || state == "child_waiting" {
					addControlChild(t, s, c.RunID(), "child")
					if state == "child_waiting" {
						child, err := r.Claim(ctx, "child")
						if err != nil {
							t.Fatal(err)
						}
						run, err := r.Load(ctx, child)
						if err != nil {
							t.Fatal(err)
						}
						if err = r.Wait(ctx, child, run.Version, "external_event"); err != nil {
							t.Fatal(err)
						}
					}
					if err := executeTaskRun(t, a, r, ctx, c); err != nil {
						t.Fatal(err)
					}
				}
			}
			assertCompleteRejected(t, s, r, a, "complete")
			if scalar(t, s, `SELECT count(*) FROM run WHERE kind='worker' AND desired_state!='running'`) != 0 && state != "paused" && state != "cancelled" {
				t.Fatal("completion attempted to stop work")
			}
		})
	}
}

func TestTaskCompleteRejectsUnresolvedInvocationsOnTerminalRoot(t *testing.T) {
	for _, state := range []string{"queued", "running", "waiting", "unknown"} {
		t.Run(state, func(t *testing.T) {
			s, r, a := controlPolicyFixture(t, nil, "ongoing")
			ctx := context.Background()
			controlMain(t, r, a, "create", "create")
			c := rootClaim(t, s, r)
			run, err := r.Load(ctx, c)
			if err != nil {
				t.Fatal(err)
			}
			g, err := r.Generation(ctx, c, run)
			if err != nil {
				t.Fatal(err)
			}
			response := calls("unavailable_external", `{}`)
			if err = s.Models().RecordUsageAnd(ctx, g.Request, response, 0, nil, s.AcceptGeneration); err != nil {
				t.Fatal(err)
			}
			ts := r.ToolStore(c).(*tools)
			invs, err := ts.Accept(ctx, g.Request.GenerationID, response.ToolCalls, run.Tools)
			if err != nil {
				t.Fatal(err)
			}
			if state != "queued" {
				if err = ts.Start(ctx, invs[0].ID); err != nil {
					t.Fatal(err)
				}
			}
			if state == "waiting" {
				if err = ts.Defer(ctx, invs[0].ID, "resource", time.Time{}); err != nil {
					t.Fatal(err)
				}
			}
			if state == "unknown" {
				if err = ts.Unknown(ctx, invs[0].ID); err != nil {
					t.Fatal(err)
				}
			}
			run, err = r.Load(ctx, c)
			if err != nil {
				t.Fatal(err)
			}
			if err = r.Fail(ctx, c, run.Version, "PROVIDER_UNAVAILABLE"); err != nil {
				t.Fatal(err)
			}
			assertCompleteRejected(t, s, r, a, "complete")
			if scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE invocation_id=? AND status=?`, invs[0].ID, state) != 1 {
				t.Fatal("completion changed unresolved invocation")
			}
		})
	}
}

func TestTaskCompleteInvocationReplayAndOwnerFence(t *testing.T) {
	s, r, a := controlPolicyFixture(t, nil, "ongoing")
	ctx := context.Background()
	controlMain(t, r, a, "create", "create")
	mustDrain(t, a)
	main := mustSubmit(t, a, "complete", "complete")
	c, err := r.Claim(ctx, main.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Initialize(ctx, c); err != nil {
		t.Fatal(err)
	}
	run, err := r.Load(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := r.Steering(ctx, c, run.Checkpoint.InputSequence)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range inputs {
		run.Checkpoint.InputSequence = i.Sequence
	}
	if err = r.Checkpoint(ctx, c, run.Version, run.Checkpoint); err != nil {
		t.Fatal(err)
	}
	run, err = r.Load(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	g, err := r.Generation(ctx, c, run)
	if err != nil {
		t.Fatal(err)
	}
	var ref string
	if err = s.db.QueryRow(`SELECT task_ref FROM task`).Scan(&ref); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(task.ManageRequest{Operation: "complete", TaskRef: ref})
	response := calls(task.ManageName, string(args))
	if err = s.Models().RecordUsageAnd(ctx, g.Request, response, 0, nil, s.AcceptGeneration); err != nil {
		t.Fatal(err)
	}
	old := r.ToolStore(c).(*tools)
	invs, err := old.Accept(ctx, g.Request.GenerationID, response.ToolCalls, run.Tools)
	if err != nil {
		t.Fatal(err)
	}
	if err = old.Start(ctx, invs[0].ID); err != nil {
		t.Fatal(err)
	}
	if err = r.Release(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err = old.ExecuteBuiltin(ctx, invs[0]); !errors.Is(err, agent.ErrClaimLost) {
		t.Fatal("stale Main owner completed Task", err)
	}
	newClaim, err := r.Claim(ctx, main.RunID)
	if err != nil {
		t.Fatal(err)
	}
	current := r.ToolStore(newClaim).(*tools)
	first, err := current.ExecuteBuiltin(ctx, invs[0])
	if err != nil || first.Error != nil {
		t.Fatal(first, err)
	}
	version := scalar(t, s, `SELECT version FROM task`)
	second, err := current.ExecuteBuiltin(ctx, invs[0])
	if err != nil || second.Text != first.Text {
		t.Fatal("invocation replay drift", second, err)
	}
	if scalar(t, s, `SELECT version FROM task`) != version || scalar(t, s, `SELECT count(*) FROM task_invocation WHERE invocation_id=?`, invs[0].ID) != 1 {
		t.Fatal("invocation replay repeated completion")
	}
}
