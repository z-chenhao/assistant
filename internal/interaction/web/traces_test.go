package web

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/observability"
)

type traceReader struct{}

func (traceReader) Traces(context.Context, observability.TraceQuery) (observability.Traces, error) {
	return observability.Traces{Items: []observability.Trace{{Execution: observability.Execution{ID: "root-public", Role: "root", Status: "completed", CreatedAt: "2026-10-29T01:00:00Z", EndedAt: "2026-10-29T01:00:01Z"}, Title: "Review report", Session: "conversation:owner", Model: "fixture-model"}}, Next: "next-public"}, nil
}
func (traceReader) Observations(_ context.Context, q observability.ObservationQuery) (observability.ObservationTree, error) {
	step := int64(0)
	end := int64(1793235601000000)
	root := observability.Observation{ID: "agent:root-public", Key: "root-public", RunID: "root-public", Name: "Root Worker", Type: "agent", Status: "completed", Started: end - 1000000, Ended: &end, Timing: "lifecycle"}
	gen := observability.Observation{ID: "generation:g", Key: "g", RunID: "root-public", Name: "Generate response", Type: "generation", Parent: root.ID, Step: &step, Status: "completed", Started: end - 800000, Ended: &end, Timing: "admission to acceptance"}
	tree := observability.ObservationTree{Trace: observability.Trace{Execution: observability.Execution{ID: "root-public", Role: "root", Root: "root-public", Status: "completed", CreatedAt: "2026-10-29T01:00:00Z", EndedAt: "2026-10-29T01:00:01Z"}, Title: "Review report", Session: "conversation:owner", Model: "fixture-model"}, Items: []observability.Observation{root, gen}, Selected: root}
	if q.Selected == gen.ID {
		tree.Selected = gen
	}
	return tree, nil
}
func (traceReader) ObservationBody(context.Context, observability.BodyQuery) (observability.Body, error) {
	return observability.Body{Available: true, Content: `{"text":"Accepted <script>unsafe()</script>","tool_calls":[]}`, Length: 68}, nil
}
func (traceReader) ChatPreview(context.Context, observability.ObservationQuery) ([]observability.ChatPreview, error) {
	return []observability.ChatPreview{{Index: 0, Role: "system", Text: "Saved system prompt", Source: "safety"}, {Index: 1, Role: "user", Text: "Private <img src=x>", Source: "conversation"}}, nil
}

func TestTraceWorkbenchAuthenticatedCompleteEscapedAndValidated(t *testing.T) {
	s := &Server{Core: &updatesCore{allowed: true, status: "idle"}, Secrets: fakeSecrets{}, Inspector: &inspectionReader{}, Traces: traceReader{}}
	request := func(path string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8080"+path, nil)
		if auth {
			r.Header.Set("Authorization", "Bearer private-token-at-least-24-bytes")
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/developer/observability?run=root-public&obs=generation:g", "/api/developer/traces", "/api/developer/observations?run=root-public", "/api/developer/observation-body?run=root-public&obs=generation:g"} {
		if w := request(path, false); w.Code != 401 {
			t.Fatal("trace auth", path, w.Code)
		}
		if w := request(path, true); w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	w := request("/developer/observability?run=root-public&obs=generation:g", true)
	for _, text := range []string{"Trace tree", "Timeline", "Observation details", "Saved system prompt", "Private &lt;img", "Accepted &lt;script&gt;", "</html>"} {
		if !strings.Contains(w.Body.String(), text) {
			t.Fatal("missing complete rendered view", text, w.Body.String())
		}
	}
	if strings.Contains(w.Body.String(), "<script>unsafe()") || strings.Contains(w.Body.String(), "<img src=x") {
		t.Fatal("context executed as HTML")
	}
	for _, path := range []string{"/developer/observability?run=x&tab=bad", "/api/developer/observations", "/api/developer/observation-body?run=x&obs=x&field=checkpoint", "/developer/observability?run=x&offset=-1", "/developer/observability?role=main&role=root"} {
		if w := request(path, true); w.Code != 400 {
			t.Fatal("invalid trace query accepted", path, w.Code)
		}
	}
}

func TestTraceRowsGroupActualParentsAndBoundWaterfall(t *testing.T) {
	tree := observability.ObservationTree{Trace: observability.Trace{Execution: observability.Execution{CreatedAt: "2026-10-29T01:00:00Z", EndedAt: "2026-10-29T01:00:01Z"}}, Items: []observability.Observation{{ID: "root"}, {ID: "a", Parent: "root"}, {ID: "b", Parent: "root"}, {ID: "grandchild", Parent: "a"}}}
	rows := traceRows(tree)
	if len(rows) != 4 || rows[2].ID != "grandchild" || rows[3].ID != "b" {
		t.Fatal(rows)
	}
	for _, r := range rows {
		if r.X < 0 || r.X+r.Width > 1000 {
			t.Fatal("invalid waterfall geometry", r)
		}
	}
	tree.Items = []observability.Observation{{ID: "fragment", Parent: "outside"}}
	rows = traceRows(tree)
	if len(rows) != 1 || !rows[0].ParentOutside || rows[0].Depth != 0 {
		t.Fatal(rows)
	}
	tree.Items = []observability.Observation{{ID: "a", Parent: "b"}, {ID: "b", Parent: "a"}}
	if len(traceRows(tree)) != 2 {
		t.Fatal("cycle escaped bounded rendering")
	}
}
