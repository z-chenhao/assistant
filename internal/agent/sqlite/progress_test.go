package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/task"
)

func TestRootLoopPublishesStageUpdatesWithoutMainWork(t *testing.T) {
	s, r, a, p := taskFixture(t)
	step, version := 0, 0
	p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
		if !isRoot(req) {
			if isFollowup(req) || req.Messages[len(req.Messages)-1].Role == "tool" {
				return llm.GenerationResult{Text: "Main answer"}, nil
			}
			result := calls(task.ManageName, `{"operation":"create","task":"Compare venue constraints"}`)
			result.Text = "MAIN_PRIVATE_TEXT"
			return result, nil
		}
		if step > 0 {
			want := "Constraints reviewed; comparing available venues."
			if step == 4 {
				want = "Comparison saved; checking the recommendation."
			}
			assertProgress(t, s, want)
			current := scalar(t, s, `SELECT version FROM task`)
			if step == 1 {
				version = current
			} else if (step == 2 || step == 3) && current != version {
				t.Fatal("identical progress churned Task version", current, version)
			} else if step == 4 && current != version+1 {
				t.Fatal("new stage did not advance Task version once", current, version)
			}
			if scalar(t, s, `SELECT count(*) FROM task_update`) != 0 || scalar(t, s, `SELECT count(*) FROM main_inbox`) != 0 {
				t.Fatal("ordinary progress notified or woke Main")
			}
			items, err := r.Activity(context.Background())
			if err != nil || len(items) != 1 || !strings.Contains(items[0].ProgressSummary, "Working. Last update: "+want) {
				t.Fatal("activity did not show current phase and semantic summary", items, err)
			}
		}
		step++
		if step == 5 {
			return llm.GenerationResult{Text: "FINAL_BUSINESS_RESULT"}, nil
		}
		result := calls("mcp__fixture__echo", `{"text":"PRIVATE_TOOL_PAYLOAD"}`)
		result.Text = "Constraints reviewed; comparing available venues."
		if step == 3 {
			result.Text = strings.Repeat("界", 171)
		}
		if step == 4 {
			result.Text = "Comparison saved; checking the recommendation."
		}
		return result, nil
	}
	mustSubmit(t, a, "stages", "Delegate this comparison")
	mustDrain(t, a)
	assertProgress(t, s, "Comparison saved; checking the recommendation.")
	items, err := r.Activity(context.Background())
	if err != nil || len(items) != 1 || !strings.HasPrefix(items[0].ProgressSummary, "Completed.") {
		t.Fatal(items, err)
	}
	if scalar(t, s, `SELECT count(*) FROM task_update`) != 1 || scalar(t, s, `SELECT count(*) FROM main_inbox`) != 1 || scalar(t, s, `SELECT count(*) FROM delivery_request`) != 2 {
		t.Fatal("stage updates created extra reports or deliveries")
	}
}

func assertProgress(t *testing.T, s *Store, want string) {
	t.Helper()
	var text string
	if err := s.db.QueryRow(`SELECT progress_summary FROM task`).Scan(&text); err != nil || text != want {
		t.Fatalf("progress: got %q want %q: %v", text, want, err)
	}
}

func executeTaskRunID(t *testing.T, a agent.Actor, r *Runs, id string) error {
	t.Helper()
	c, err := r.Claim(context.Background(), id)
	if err != nil {
		return err
	}
	return executeTaskRun(t, a, r, context.Background(), c)
}

func TestProgressCheckpointAtomicRecoveryAndOwnership(t *testing.T) {
	s, r, a, p := taskFixture(t)
	p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
		if isRoot(req) {
			if req.Messages[len(req.Messages)-1].Role == "tool" {
				return llm.GenerationResult{Text: "Business result"}, nil
			}
			result := calls("mcp__fixture__echo", `{"text":"evidence"}`)
			result.Text = "Sources checked; preparing the comparison."
			return result, nil
		}
		if req.Messages[len(req.Messages)-1].Role == "tool" {
			return llm.GenerationResult{Text: "Accepted"}, nil
		}
		return calls(task.ManageName, `{"operation":"create","task":"Compare sources"}`), nil
	}
	main := mustSubmit(t, a, "atomic", "Delegate")
	if err := executeTaskRunID(t, a, r, main.RunID); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := s.db.QueryRow(`SELECT run_id FROM run WHERE worker_position='root'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	c, err := r.Claim(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`CREATE TRIGGER fail_progress BEFORE UPDATE OF progress_summary ON task BEGIN SELECT RAISE(ABORT,'progress unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if err = executeTaskRun(t, a, r, ctx, c); !errors.Is(err, agent.ErrStorage) {
		t.Fatal("checkpoint must fail with progress transaction", err)
	}
	assertProgress(t, s, "")
	if scalar(t, s, `SELECT count(*) FROM run WHERE run_id=? AND json_extract(checkpoint_json,'$.phase')='model'`, id) != 1 || scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE run_id=?`, id) != 0 {
		t.Fatal("progress failure left a partial checkpoint or executed tools")
	}
	if scalar(t, s, `SELECT count(*) FROM model_generation WHERE run_id=? AND status='completed'`, id) != 1 {
		t.Fatal("complete model response was lost")
	}
	if _, err = s.db.Exec(`DROP TRIGGER fail_progress`); err != nil {
		t.Fatal(err)
	}
	// A different owner recovers the durable generation after the failed commit.
	if err = executeTaskRunID(t, a, r, id); err != nil {
		t.Fatal(err)
	}
	assertProgress(t, s, "Sources checked; preparing the comparison.")
	if scalar(t, s, `SELECT count(*) FROM model_generation WHERE run_id=?`, id) != 2 || scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE run_id=?`, id) != 1 {
		t.Fatal("recovery repeated the model turn or tool")
	}
	// The old owner cannot overwrite a new owner's committed projection.
	if err = r.Progress(ctx, c, "stale"); !errors.Is(err, agent.ErrClaimLost) {
		t.Fatal("old owner updated progress", err)
	}
}

func TestProgressExcludesChildrenLegacyAndInvalidProse(t *testing.T) {
	for _, tc := range []struct {
		name, text, want string
		clipped, legacy  bool
	}{
		{name: "empty"},
		{name: "whitespace", text: " \n\t "},
		{name: "too long", text: strings.Repeat("界", 171)},
		{name: "clipped", text: "incomplete", clipped: true},
		{name: "terminal controls", text: "\x1b[2JHidden instructions"},
		{name: "legacy", text: "LEGACY_PRIVATE_TEXT", legacy: true},
		{name: "bounded", text: strings.Repeat("a", 512), want: strings.Repeat("a", 512)},
		{name: "normalized", text: "已核对来源。\n\t 正在比较。", want: "已核对来源。 正在比较。"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, r, a, p := childFixture(t)
			p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
				last := req.Messages[len(req.Messages)-1]
				if last.Role == "tool" || isFollowup(req) {
					return llm.GenerationResult{Text: "Business result"}, nil
				}
				if !isRoot(req) {
					return calls(task.ManageName, `{"operation":"create","task":"Root request"}`), nil
				}
				if last.Text == "Child request" {
					result := calls("mcp__fixture__echo", `{"text":"PRIVATE_CHILD_TOOL_DATA"}`)
					result.Text = "PRIVATE_CHILD_PROGRESS"
					return result, nil
				}
				result := calls(agent.RunToolName, `{"task":"Child request"}`)
				result.Text, result.Clipped = tc.text, tc.clipped
				return result, nil
			}
			main := mustSubmit(t, a, "prose", "Delegate")
			if err := executeTaskRunID(t, a, r, main.RunID); err != nil {
				t.Fatal(err)
			}
			if tc.legacy {
				// Construct the pre-feature frozen Root context, with no opt-in.
				var id string
				if err := s.db.QueryRow(`SELECT run_id FROM run WHERE worker_position='root'`).Scan(&id); err != nil {
					t.Fatal(err)
				}
				ctx := context.Background()
				claim, err := r.Claim(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				run, err := r.Load(ctx, claim)
				if err != nil {
					t.Fatal(err)
				}
				cp := run.Checkpoint
				messages := []agent.AgentMessage{}
				for _, m := range cp.Messages {
					if m.Source == nil || m.Source.Kind != "progress_guidance" {
						messages = append(messages, m)
					}
				}
				cp.Messages = messages
				if err := r.Checkpoint(ctx, claim, run.Version, cp); err != nil {
					t.Fatal(err)
				}
				if err := r.Release(ctx, claim); err != nil {
					t.Fatal(err)
				}
			}
			mustDrain(t, a)
			assertProgress(t, s, tc.want)
			if scalar(t, s, `SELECT count(*) FROM run WHERE worker_position='child' AND status='completed'`) != 1 {
				t.Fatal("did not exercise an actual Child loop")
			}
		})
	}
}
