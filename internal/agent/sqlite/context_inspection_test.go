package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/observability"
)

// Compare the complete inspection projection with the actual admitted records,
// including semantic-summary rounds. Observation must not rebuild from a newer
// checkpoint, drop tool calls/results or change the frozen hashes.
func assertFrozenContextRun(t *testing.T, s *Store, id string) {
	t.Helper()
	ctx := context.Background()
	reader := s.Inspector()
	rounds, err := reader.Rounds(ctx, observability.RoundQuery{RunID: id, After: -1})
	if err != nil || len(rounds.Items) == 0 {
		t.Fatal("rounds", err)
	}
	for _, round := range rounds.Items {
		var request, result string
		if err = s.db.QueryRow(`SELECT request_json,coalesce(result_json,'null') FROM model_generation WHERE run_id=? AND step_index=?`, id, round.Step).Scan(&request, &result); err != nil {
			t.Fatal(err)
		}
		var frozen struct {
			Messages []json.RawMessage `json:"messages"`
			Tools    []json.RawMessage `json:"tools"`
		}
		if err = json.Unmarshal([]byte(request), &frozen); err != nil {
			t.Fatal(err)
		}
		sections := map[string][]json.RawMessage{"messages": frozen.Messages, "tools": frozen.Tools}
		var provenance, hits string
		var sectionsRecall []json.RawMessage
		if err = s.db.QueryRow(`SELECT provenance_json FROM context_compaction WHERE run_id=? AND step_index=?`, id, round.Step).Scan(&provenance); err == nil {
			sections["provenance"] = []json.RawMessage{json.RawMessage(provenance)}
		} else if err != sql.ErrNoRows {
			t.Fatal(err)
		}
		if err = s.db.QueryRow(`SELECT hits_json FROM run_recall WHERE run_id=? AND step_index=?`, id, round.Step).Scan(&hits); err == nil {
			if json.Unmarshal([]byte(hits), &sectionsRecall) != nil {
				t.Fatal("invalid frozen recall")
			}
			sections["recall"] = sectionsRecall
		} else if err != sql.ErrNoRows {
			t.Fatal(err)
		}
		if round.Status == "completed" {
			sections["response"] = []json.RawMessage{json.RawMessage(result)}
		}
		for section, items := range sections {
			for index, expected := range items {
				q := observability.ContextQuery{RunID: id, Step: round.Step, Section: section, Item: index, JSON: true}
				var complete strings.Builder
				for {
					part, e := reader.Context(ctx, q)
					if e != nil {
						t.Fatal(section, index, e)
					}
					complete.WriteString(part.Content)
					if part.NextOffset == nil {
						break
					}
					q.Offset = *part.NextOffset
				}
				var want, got any
				if json.Unmarshal(expected, &want) != nil || json.Unmarshal([]byte(complete.String()), &got) != nil || !reflect.DeepEqual(want, got) {
					t.Fatal("inspection differs from frozen input/result", section, index)
				}
			}
		}
	}
}

func TestContextInspectionRealMainRootChildAndReplayPreservesSnapshots(t *testing.T) {
	s, _, a, p := childFixture(t)
	p.respond = childPolicy
	mustSubmit(t, a, "context-inspection", "Private request visible only in authorized context")
	mustDrain(t, a)
	counts := map[string]int{}
	for _, table := range []string{"run", "tool_invocation", "model_generation", "run_trace", "event_outbox", "context_compaction"} {
		counts[table] = countRows(t, s, table)
	}
	ids := map[string]bool{}
	for _, request := range p.requests {
		ids[request.RunID] = true
	}
	if len(ids) < 4 {
		t.Fatal("recursive execution fixture incomplete")
	}
	for id := range ids {
		assertFrozenContextRun(t, s, id)
	}
	mustDrain(t, a)
	for table, before := range counts {
		if after := countRows(t, s, table); after != before {
			t.Fatal("read or repeated drain changed domain state", table, before, after)
		}
	}
}
