package web

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/observability"
)

type inspectionReader struct {
	failure error
	queries []observability.ExecutionQuery
}

func TestInspectionTreeGroupsParallelBranchesAndMarksPageFragments(t *testing.T) {
	items := []observability.Execution{{ID: "root", Role: "root"}, {ID: "a", Parent: "root", Role: "child", Depth: 1}, {ID: "b", Parent: "root", Role: "child", Depth: 1}, {ID: "grandchild", Parent: "a", Role: "child", Depth: 2}}
	tree := inspectionTree(items)
	if len(tree) != 4 || tree[0].ID != "root" || tree[1].ID != "a" || tree[2].ID != "grandchild" || tree[3].ID != "b" || tree[2].DisplayDepth != 2 {
		t.Fatal("parallel branch guessed from creation order", tree)
	}
	fragment := inspectionTree(items[3:])
	if len(fragment) != 1 || !fragment[0].ParentOutside || fragment[0].Parent != "a" || fragment[0].DisplayDepth != 0 {
		t.Fatal("missing parent not marked", fragment)
	}
	cycle := inspectionTree([]observability.Execution{{ID: "a", Parent: "b"}, {ID: "b", Parent: "a"}})
	if len(cycle) != 2 {
		t.Fatal("invalid metadata broke bounded rendering")
	}
}

func (f *inspectionReader) Executions(_ context.Context, q observability.ExecutionQuery) (observability.Executions, error) {
	f.queries = append(f.queries, q)
	return observability.Executions{Items: []observability.Execution{{ID: "main-public", Role: "main", Status: "completed", CreatedAt: "2026-10-28T01:00:00Z"}}, Next: "page-public"}, f.failure
}
func (f *inspectionReader) Relationships(_ context.Context, q observability.RelationshipQuery) (observability.Relationships, error) {
	return observability.Relationships{Selected: &observability.Execution{ID: q.RunID, Role: "root", Status: "completed", Root: q.RunID}, Task: &observability.TaskNode{Ref: "task:public", Title: `<img src=x onerror="unsafe()">`, Status: "completed"}, Creator: &observability.Execution{ID: "main-public", Role: "main"}, Executions: []observability.Execution{{ID: q.RunID, Role: "root", Root: q.RunID, Epoch: 1}}}, f.failure
}

func TestInspectionRoutesAuthenticationReadOnlyBoundedAndEscaped(t *testing.T) {
	reader := &inspectionReader{}
	s := &Server{Core: &updatesCore{allowed: true, status: "idle"}, Secrets: fakeSecrets{}, Inspector: reader}
	request := func(method, path string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, nil)
		if auth {
			r.Header.Set("Authorization", "Bearer private-token-at-least-24-bytes")
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/developer/observability", "/api/developer/executions", "/api/developer/relationships?run=root-public"} {
		if w := request("GET", path, false); w.Code != 401 {
			t.Fatal("unauthenticated inspector", path, w.Code)
		}
	}
	if len(reader.queries) != 0 {
		t.Fatal("unauthenticated query reached storage")
	}
	if w := request("GET", "/api/developer/executions?limit=1&role=child&status=completed&since=2026-10-28T01:00:00%2B08:00", true); w.Code != 200 || len(reader.queries) != 1 || reader.queries[0].Since.Format("2006-01-02T15:04:05Z07:00") != "2026-10-27T17:00:00Z" {
		t.Fatal(w.Code, w.Body.String(), reader.queries)
	}
	w := request("GET", "/developer/observability?run=root-public", true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, text := range []string{"Developer", "Execution relationships", "Task root · no parent Worker", "Created by", "Main", "&lt;img src=x", "Task execution tree", "Older executions"} {
		if !strings.Contains(w.Body.String(), text) {
			t.Fatal("missing inspector behavior", text)
		}
	}
	if strings.Contains(w.Body.String(), `<img src=x`) || strings.Contains(w.Body.String(), `id="composer-root"`) {
		t.Fatal("unsafe title or conversation dependency in inspector")
	}
	for _, path := range []string{"/api/developer/executions?limit=101", "/api/developer/executions?limit=0", "/api/developer/executions?role=worker", "/api/developer/executions?role=main&role=child", "/api/developer/executions?since=bad", "/api/developer/relationships", "/developer/observability?run=a&task=b"} {
		if w := request("GET", path, true); w.Code != 400 {
			t.Fatal("invalid query accepted", path, w.Code)
		}
	}
	if w := request("POST", "/api/developer/executions", true); w.Code != 404 {
		t.Fatal("inspector gained action route", w.Code)
	}
	reader.failure = observability.ErrNotFound
	if w := request("GET", "/api/developer/relationships?run=missing", true); w.Code != 404 {
		t.Fatal(w.Code)
	}
	reader.failure = errors.New("PRIVATE_SQL_DIAGNOSTIC")
	w = request("GET", "/api/developer/executions", true)
	if w.Code != 503 || strings.Contains(w.Body.String(), "PRIVATE_SQL") {
		t.Fatal("storage diagnostic exposed", w.Code, w.Body.String())
	}
}
