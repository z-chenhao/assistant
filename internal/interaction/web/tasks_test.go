package web

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/task"
)

type workspaceReader struct{ err error }

func (f workspaceReader) List(context.Context, task.ListQuery) (task.List, error) {
	return task.List{Items: []task.Entry{{Summary: task.Summary{TaskRef: "task:visible", Title: "Review report", Status: "active"}, Stage: "Working", CompletionPolicy: "ongoing"}}}, f.err
}
func (f workspaceReader) Detail(context.Context, string, string) (task.Detail, error) {
	return task.Detail{Entry: task.Entry{Summary: task.Summary{TaskRef: "task:visible", Title: "Review <img src=x>", GoalSummary: "Prepare a review", Status: "active"}, Stage: "Working", CompletionPolicy: "ongoing", Progress: task.Progress{Running: 1, Succeeded: 2, Waiting: 1}}, SourceSequence: 8, Instructions: []task.Instruction{{Text: "Use the primary sources", At: "2026-11-03T00:00:00Z", Epoch: 1}}}, f.err
}

func (f workspaceReader) Scheduled(context.Context, string) ([]task.ScheduledWork, error) {
	return []task.ScheduledWork{{Schedule: task.Schedule{Ref: "automation:next", Instruction: "Prepare tomorrow <script>", Type: "cron", Spec: "0 8 * * *", Timezone: "Asia/Shanghai", Enabled: true, NextAt: "2026-11-04T00:00:00Z"}}}, f.err
}

type unavailableScheduleReader struct{ workspaceReader }

func (unavailableScheduleReader) Scheduled(context.Context, string) ([]task.ScheduledWork, error) {
	return nil, errors.New("PRIVATE_SCHEDULE_ERROR")
}

func TestTaskPagesAreAuthenticatedReadOnlyAndErrorsStayPrivate(t *testing.T) {
	s := &Server{Config: Config{TokenRef: "credential:test"}, Core: &updatesCore{allowed: true, status: "idle"}, Tasks: workspaceReader{}, Secrets: fakeSecrets{}}
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
	list := request("/tasks", true)
	for _, text := range []string{"Scheduled work", "Not dispatched", "Prepare tomorrow &lt;script&gt;", "Next scheduled occurrence", "Asia/Shanghai", "0 8 * * *"} {
		if !strings.Contains(list.Body.String(), text) {
			t.Fatal("missing saved schedule", text)
		}
	}
	if strings.Contains(list.Body.String(), "/tasks/automation:") {
		t.Fatal("schedule became an invented task")
	}
	for _, view := range []string{"closed", "attention"} {
		w := request("/tasks?view="+view, true)
		if w.Code != 200 || strings.Contains(w.Body.String(), "Prepare tomorrow") || strings.Contains(w.Body.String(), "<h2>Scheduled work</h2>") {
			t.Fatal("schedule escaped task view", view, w.Code)
		}
	}
	s.Tasks = unavailableScheduleReader{}
	unavailable := request("/tasks", true)
	if unavailable.Code != 200 || !strings.Contains(unavailable.Body.String(), "Schedules are temporarily unavailable") || !strings.Contains(unavailable.Body.String(), "Review report") || strings.Contains(unavailable.Body.String(), "PRIVATE_SCHEDULE_ERROR") {
		t.Fatal("schedule read masked tasks or leaked diagnostics", unavailable.Code, unavailable.Body.String())
	}
	s.Tasks = workspaceReader{}
	w := request("/tasks/task:visible", true)
	for _, text := range []string{"Review &lt;img src=x&gt;", "Continue this task", "before=9", "Related updates from Ghost", "Delegated work · 4 total", "Completed / failed", "Running / queued / waiting", "Planned objective", "Ongoing task", "Accepted instructions", "Use the primary sources", "Delegated work"} {
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
