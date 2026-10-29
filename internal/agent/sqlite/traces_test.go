package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/observability"
)

func TestTraceReadModelMainRootChildUsageAndBodies(t *testing.T) {
	ctx := context.Background()
	s, _, a, p := childFixture(t)
	p.respond = childPolicy
	main := mustSubmit(t, a, "trace-model", "Main input <img src=x>")
	mustDrain(t, a)
	reader := s.Inspector()
	before := countRows(t, s, "run_trace")
	list, err := reader.Traces(ctx, observability.TraceQuery{})
	if err != nil || len(list.Items) < 2 {
		t.Fatal(list, err)
	}
	var root string
	for _, trace := range list.Items {
		if trace.Role == "root" {
			root = trace.ID
		}
		if trace.Role == "child" {
			t.Fatal("child became independent trace")
		}
		if trace.Metrics.Generations == 0 {
			t.Fatal("missing generations")
		}
	}
	tree, err := reader.Observations(ctx, observability.ObservationQuery{RunID: root})
	if err != nil {
		t.Fatal(err)
	}
	if tree.Trace.Parent != "" || tree.Trace.Root != root || tree.Selected.Type != "agent" {
		t.Fatal("trace ancestry changed", tree)
	}
	kinds := map[string]int{}
	gens := map[string]bool{}
	var toolID string
	for _, node := range tree.Items {
		kinds[node.Type]++
		if node.Type == "generation" {
			gens[node.Key] = true
			if node.Parent != "agent:"+node.RunID {
				t.Fatal("generation parent", node)
			}
			out, e := reader.ObservationBody(ctx, observability.BodyQuery{ObservationQuery: observability.ObservationQuery{RunID: root, Selected: node.ID}, Field: "output"})
			if e != nil || !out.Available || !json.Valid([]byte(out.Content)) {
				t.Fatal(out, e)
			}
			messages, e := reader.ChatPreview(ctx, observability.ObservationQuery{RunID: root, Selected: node.ID})
			if e != nil || len(messages) == 0 {
				t.Fatal(messages, e)
			}
			wire, e := reader.ObservationBody(ctx, observability.BodyQuery{ObservationQuery: observability.ObservationQuery{RunID: root, Selected: node.ID}, Field: "wire"})
			if e != nil || wire.Available {
				t.Fatal("missing dispatch inferred", wire, e)
			}
		}
		if node.Type == "tool" {
			toolID = node.ID
		}
	}
	if kinds["agent"] != 3 || kinds["generation"] < 3 || kinds["tool"] < 2 || kinds["chain"] < 3 {
		t.Fatal(kinds)
	}
	if int64(len(gens)) != tree.Trace.Metrics.Generations {
		t.Fatal("generation counted twice", len(gens), tree.Trace.Metrics)
	}
	if _, e := reader.Observations(ctx, observability.ObservationQuery{RunID: main.RunID, Selected: toolID}); !errors.Is(e, observability.ErrNotFound) {
		t.Fatal("cross-trace selection revealed", e)
	}
	if _, e := reader.ObservationBody(ctx, observability.BodyQuery{ObservationQuery: observability.ObservationQuery{RunID: main.RunID, Selected: toolID}, Field: "input"}); !errors.Is(e, observability.ErrNotFound) {
		t.Fatal("cross-trace body revealed", e)
	}
	selected, e := reader.Observations(ctx, observability.ObservationQuery{RunID: root, Selected: toolID})
	if e != nil || selected.Selected.ID != toolID {
		t.Fatal(selected, e)
	}
	body, e := reader.ObservationBody(ctx, observability.BodyQuery{ObservationQuery: observability.ObservationQuery{RunID: root, Selected: toolID}, Field: "input"})
	if e != nil || !body.Available {
		t.Fatal(body, e)
	}
	if strings.Contains(body.Content, "owner_token") || strings.Contains(body.Content, "model_snapshot") {
		t.Fatal("internal ownership/config leaked")
	}
	if countRows(t, s, "run_trace") != before {
		t.Fatal("read wrote observation")
	}
}

func TestTraceObservationPaginationHasStableScopedKeys(t *testing.T) {
	ctx := context.Background()
	s, _, a, p := childFixture(t)
	p.respond = childPolicy
	main := mustSubmit(t, a, "trace-pagination", "read-only inspection")
	mustDrain(t, a)
	reader := s.Inspector()
	initial, err := reader.Observations(ctx, observability.ObservationQuery{RunID: main.RunID})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 220; i++ {
		if _, err = s.db.Exec(`INSERT INTO run_trace VALUES(?,?,?,100)`, main.RunID, 10000+i, `{"kind":"agent_start","step_index":0}`); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for range 10 {
		page, e := reader.Observations(ctx, observability.ObservationQuery{RunID: main.RunID, Cursor: cursor})
		if e != nil {
			t.Fatal(e)
		}
		if len(page.Items) > 100 {
			t.Fatal("unbounded observations")
		}
		for _, o := range page.Items {
			if seen[o.ID] {
				t.Fatal("duplicate observation")
			}
			seen[o.ID] = true
		}
		if page.Next == "" {
			break
		}
		cursor = page.Next
	}
	if len(seen) != len(initial.Items)+220 {
		t.Fatal("pagination omitted rows", len(seen))
	}
	var root string
	if err = s.db.QueryRow(`SELECT run_id FROM run WHERE worker_position='root'`).Scan(&root); err != nil {
		t.Fatal(err)
	}
	if _, e := reader.Observations(ctx, observability.ObservationQuery{RunID: root, Cursor: cursor}); !errors.Is(e, observability.ErrInvalid) {
		t.Fatal("cursor crossed trace", e)
	}
}
