package web

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/observability"
)

type frozenReader struct{ failure error }

func (f frozenReader) Rounds(context.Context, observability.RoundQuery) (observability.Rounds, error) {
	return observability.Rounds{Items: []observability.Round{{Step: 0, ID: "first", Status: "completed"}, {Step: 1, ID: "next", Status: "completed"}}}, f.failure
}
func (f frozenReader) Context(context.Context, observability.ContextQuery) (observability.FrozenContext, error) {
	previous := int64(0)
	next := 8192
	item := observability.ContextItem{Index: 0, Label: "system", SourceRef: "trace:recorded", SourceKind: "system"}
	return observability.FrozenContext{Round: observability.Round{Step: 1, ID: "next", Status: "completed"}, Previous: &previous, Items: []observability.ContextItem{item}, Selected: &item, Content: `<img src=x onerror="unsafe()">PRIVATE_CONTEXT`, Length: 9000, NextOffset: &next}, f.failure
}
func (f frozenReader) ContextDiff(context.Context, observability.DiffQuery) (observability.ContextDiff, error) {
	previous := int64(0)
	index := 0
	return observability.ContextDiff{Previous: &previous, Items: []observability.ContextChange{{Kind: "changed", Before: &index, After: &index, Label: "system"}}}, f.failure
}

func TestFrozenContextAuthenticatedReadOnlyRoutesAndCompleteHTML(t *testing.T) {
	s := &Server{Core: &updatesCore{allowed: true, status: "idle"}, Secrets: fakeSecrets{}, Contexts: frozenReader{}}
	request := func(method, path string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, nil)
		if auth {
			r.Header.Set("Authorization", "Bearer private-token-at-least-24-bytes")
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/developer/context?run=run&step=1", "/api/developer/rounds?run=run", "/api/developer/context?run=run&step=1", "/api/developer/context-diff?run=run&step=1"} {
		if w := request("GET", path, false); w.Code != 401 || strings.Contains(w.Body.String(), "PRIVATE_CONTEXT") {
			t.Fatal("unauthenticated context", w.Code)
		}
		if w := request("GET", path, true); w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
		if w := request("POST", path, true); w.Code != 404 {
			t.Fatal("context action exposed", w.Code)
		}
	}
	w := request("GET", "/developer/context?run=run&step=1", true)
	for _, text := range []string{"Frozen context", "Step 0", "system", "PRIVATE_CONTEXT", "&lt;img src=x", "trace:recorded", "Continue full content", "Adjacent round diff", "Before [0]", "After [0]", "</html>"} {
		if !strings.Contains(w.Body.String(), text) {
			t.Fatal("missing complete rendered page", text, w.Body.String())
		}
	}
	if strings.Contains(w.Body.String(), `<img src=x`) || strings.Contains(w.Body.String(), `id="composer-root"`) {
		t.Fatal("unsafe content or hidden conversation dependency")
	}
	if w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatal("content access boundary changed")
	}
	for _, path := range []string{"/developer/context", "/developer/context?run=run&step=-1", "/api/developer/context?run=run", "/api/developer/context-diff?run=run&step=1&section=response", "/api/developer/context?run=run&step=1&item=-1", "/api/developer/context?run=run&step=1&offset=1073741825", "/api/developer/context?run=run&step=1&section=model_snapshot", "/api/developer/context?run=run&step=1&run=other", "/api/developer/context?run=run&step=1&format=raw", "/api/developer/rounds?run=run&step=1"} {
		if w := request("GET", path, true); w.Code != 400 {
			t.Fatal("invalid query accepted", path, w.Code, w.Body.String())
		}
	}
	s.Contexts = frozenReader{failure: observability.ErrNotFound}
	if w := request("GET", "/api/developer/context?run=missing&step=0", true); w.Code != 404 {
		t.Fatal(w.Code)
	}
	s.Contexts = frozenReader{failure: errors.New("PRIVATE_DATABASE_ERROR")}
	w = request("GET", "/developer/context?run=run&step=1", true)
	if w.Code != 503 || strings.Contains(w.Body.String(), "PRIVATE_DATABASE_ERROR") {
		t.Fatal("error leaked", w.Code)
	}
}

func TestContextLinksResetDependentOffsets(t *testing.T) {
	v := &contextView{Query: observability.ContextQuery{RunID: "run", Step: 1, Section: "response", Item: 5, Offset: 8192, After: 40, JSON: true}, RoundAfter: -1, DiffAfter: 40}
	r := httptest.NewRequest("GET", contextLink(v, "step", int64(0)), nil)
	next, err := contextQuery(r)
	if err != nil || next.Query.Step != 0 || next.Query.Item != 0 || next.Query.Offset != 0 || next.Query.JSON || next.Query.Section != "messages" || next.DiffAfter != 0 {
		t.Fatal(next, err)
	}
	r = httptest.NewRequest("GET", diffLink(v, 0, 41), nil)
	next, err = contextQuery(r)
	if err != nil || next.Query.After != 40 || next.Query.Item != 41 || next.Query.Offset != 0 {
		t.Fatal(next, err)
	}
}
