package sqlite

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/z-chenhao/erii/internal/task"
)

func TestTaskWorkspaceUsesOwnerScopeStablePaginationAndMainDeliveries(t *testing.T) {
	ctx := context.Background()
	s, _, a, p := childFixture(t)
	p.respond = childPolicy
	input := mustSubmit(t, a, "workspace", "Prepare a report")
	mustDrain(t, a)
	reader := s.TaskReader()
	all, err := reader.List(ctx, task.ListQuery{View: "all"})
	if err != nil || len(all.Items) != 1 {
		t.Fatal(all, err)
	}
	ref := all.Items[0].TaskRef
	before := map[string]int{}
	for _, table := range []string{"run", "task", "task_instruction", "task_update", "model_generation", "event_outbox"} {
		before[table] = scalar(t, s, "SELECT count(*) FROM "+table)
	}
	detail, err := reader.Detail(ctx, ref, "")
	if err != nil {
		t.Fatal(err)
	}
	if detail.GoalSummary == "" || detail.SourceSequence == 0 {
		t.Fatal("missing original request or goal", detail)
	}
	var source string
	if err = s.db.QueryRow(`SELECT json_extract(message_json,'$.message_id') FROM conversation_intent WHERE sequence=?`, detail.SourceSequence).Scan(&source); err != nil || source != input.MessageID {
		t.Fatal("wrong conversation deep link", source, err)
	}
	if len(detail.Messages) == 0 {
		t.Fatal("Main delivery missing from task detail")
	}
	for _, m := range detail.Messages {
		if m.Role != "assistant" || m.Source != "main" {
			t.Fatal("Worker result became a separate user delivery", m)
		}
	}
	for table, count := range before {
		if scalar(t, s, "SELECT count(*) FROM "+table) != count {
			t.Fatal("browsing mutated", table)
		}
	}
	for i := 0; i < 65; i++ {
		_, err = s.db.Exec(`INSERT INTO task(task_id,task_ref,agent_id,conversation_id,title,anchor_task,anchor_artifact_refs,status,task_epoch,created_at,updated_at,version) SELECT ?,?,agent_id,agent_id,?,'Workspace goal','[]','active',1,100,100,1 FROM agent_config WHERE singleton=1`, fmt.Sprintf("workspace-%02d", i), fmt.Sprintf("task:workspace-%02d", i), fmt.Sprintf("Workspace %02d", i))
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.db.Exec(`INSERT INTO task(task_id,task_ref,agent_id,conversation_id,title,anchor_task,anchor_artifact_refs,status,task_epoch,created_at,updated_at,version) VALUES('foreign','task:foreign','other-agent','other-agent','Private foreign task','Private goal','[]','active',1,100,100,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err = reader.Detail(ctx, "task:foreign", ""); !errors.Is(err, task.ErrNotFound) {
		t.Fatal("foreign task visible", err)
	}
	seen := map[string]bool{}
	cursor := ""
	for range 5 {
		page, e := reader.List(ctx, task.ListQuery{View: "all", Query: "Workspace", Cursor: cursor, Limit: 20})
		if e != nil {
			t.Fatal(e)
		}
		for _, item := range page.Items {
			if seen[item.TaskRef] {
				t.Fatal("unstable page duplicated a task", item.TaskRef)
			}
			seen[item.TaskRef] = true
		}
		cursor = page.Next
		if cursor == "" {
			break
		}
		if _, e = reader.List(ctx, task.ListQuery{View: "closed", Query: "Workspace", Cursor: cursor}); !errors.Is(e, task.ErrInvalid) {
			t.Fatal("cursor escaped its view", e)
		}
	}
	if len(seen) != 65 {
		t.Fatal("task history is still capped or leaks another owner", len(seen))
	}
}
