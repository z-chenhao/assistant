package sqlite

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/task"
)

func TestAutomaticRecallUsesTaskOriginalRatherThanExpandedAnchor(t *testing.T) {
	s, r, a, p := taskFixture(t)
	p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
		if isRoot(req) || isFollowup(req) || req.Messages[len(req.Messages)-1].Role == "tool" {
			return llm.GenerationResult{Text: "done"}, nil
		}
		return calls(task.ManageName, `{"operation":"create","task":"Expanded harness architecture instruction with trusted tools and unrelated policy"}`), nil
	}
	original := "帮我查一下 grokbot 的赚钱路线"
	mustSubmit(t, a, "original", original)
	mustDrain(t, a)
	var id, checkpoint string
	if e := s.db.QueryRow(`SELECT run_id,checkpoint_json FROM run WHERE kind='worker' ORDER BY created_at LIMIT 1`).Scan(&id, &checkpoint); e != nil {
		t.Fatal(e)
	}
	run := agent.Run{ID: id, Kind: "worker"}
	if e := json.Unmarshal([]byte(checkpoint), &run.Checkpoint); e != nil {
		t.Fatal(e)
	}
	q, e := r.automaticRecallQuery(context.Background(), run, []llm.Message{{Role: "user", Text: "Task anchor: Expanded harness instruction"}}, true)
	if e != nil || strings.TrimSpace(q.Query) != original || len(q.ExcludeRefs) == 0 {
		t.Fatal(q, e)
	}
	var source string
	s.db.QueryRow(`SELECT source_message_id FROM task_request LIMIT 1`).Scan(&source)
	found := false
	for _, ref := range q.ExcludeRefs {
		found = found || ref == "conversation:"+source
	}
	if !found {
		t.Fatal("original request not excluded", q)
	}
}
func TestAutomaticRecallExcludesCurrentAndUnconsumedMainInput(t *testing.T) {
	s, r, _, a, _ := loopFixture(t)
	ctx := context.Background()
	first := mustSubmit(t, a, "first", "grokbot roadmap")
	mustSubmit(t, a, "second", "later accepted input")
	q, e := r.automaticRecallQuery(ctx, agent.Run{ID: first.RunID}, []llm.Message{{Role: "user", Text: "grokbot roadmap"}}, true)
	if e != nil {
		t.Fatal(e)
	}
	rows, e := s.db.Query(`SELECT message_id FROM main_input WHERE run_id=?`, first.RunID)
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			t.Fatal(e)
		}
		found := false
		for _, ref := range q.ExcludeRefs {
			found = found || ref == "conversation:"+id
		}
		if !found {
			t.Fatal("current input not excluded", id)
		}
		n++
	}
	if n != 2 || q.TimeRange == nil || q.TimeRange.End == nil {
		t.Fatal("missing accepted input/future fence", n, q)
	}
}

func TestAutomaticRecallCanRecoverConsumedInputOutsideWorkingSet(t *testing.T) {
	s, r, _, a, _ := loopFixture(t)
	first := mustSubmit(t, a, "old", "old tea decision")
	mustSubmit(t, a, "new", "current tea question")
	var id string
	var sequence int64
	if err := s.db.QueryRow(`SELECT message_id,sequence FROM main_input WHERE run_id=? ORDER BY sequence LIMIT 1`, first.RunID).Scan(&id, &sequence); err != nil {
		t.Fatal(err)
	}
	run := agent.Run{ID: first.RunID}
	run.Checkpoint.InputSequence = sequence + 1
	q, err := r.automaticRecallQuery(context.Background(), run, []llm.Message{{Role: "user", Text: "current tea question"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range q.ExcludeRefs {
		if ref == "conversation:"+id {
			t.Fatal("compacted original cannot be recovered", q)
		}
	}
}
