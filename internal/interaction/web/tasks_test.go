package web

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/task"
)

type workspaceReader struct{ err error }

func (f workspaceReader) List(context.Context, task.ListQuery) (task.List, error) {
	return task.List{Items: []task.Entry{{Summary: task.Summary{TaskRef: "task:visible", Title: "Review report", Status: "active"}, Stage: "Working"}}}, f.err
}
func (f workspaceReader) Detail(context.Context, string, string) (task.Detail, error) {
	return task.Detail{Entry: task.Entry{Summary: task.Summary{TaskRef: "task:visible", Title: "Review <img src=x>", GoalSummary: "Prepare a review", Status: "active"}, Stage: "Working", Progress: task.Progress{Running: 1, Succeeded: 2, Waiting: 1}}, SourceSequence: 8}, f.err
}

func TestTaskPagesAreAuthenticatedReadOnlyAndErrorsStayPrivate(t *testing.T) {
	s := &Server{Core: &updatesCore{allowed: true, status: "idle"}, Tasks: workspaceReader{}, Secrets: fakeSecrets{}}
	request := func(path string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8080"+path, nil)
		if auth {
			r.Header.Set("Authorization", "Bearer private-token-at-least-24-bytes")
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/tasks", "/tasks/task:visible"} {
		if w := request(path, false); w.Code != 401 {
			t.Fatal(path, w.Code)
		}
		if w := request(path, true); w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	w := request("/tasks/task:visible", true)
	for _, text := range []string{"Review &lt;img src=x&gt;", "Continue this task", "before=9", "Related updates from Erii", "Delegated work · 4 total", "Completed / failed", "Running / queued / waiting"} {
		if !strings.Contains(w.Body.String(), text) {
			t.Fatal("missing task flow", text)
		}
	}
	if strings.Contains(w.Body.String(), "<img src=x") {
		t.Fatal("task title executed")
	}
	if w := request("/tasks?view=failed", true); w.Code != 400 {
		t.Fatal("invented Task state accepted", w.Code)
	}
	s.Tasks = workspaceReader{err: task.ErrNotFound}
	if w := request("/tasks/task:foreign", true); w.Code != 404 {
		t.Fatal(w.Code)
	}
	s.Tasks = workspaceReader{err: errors.New("PRIVATE_SQL_ERROR")}
	if w := request("/tasks", true); w.Code != 503 || strings.Contains(w.Body.String(), "PRIVATE_SQL_ERROR") {
		t.Fatal(w.Code, w.Body.String())
	}
}
