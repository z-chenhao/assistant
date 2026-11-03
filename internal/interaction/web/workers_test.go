package web

import (
	"context"
	"errors"
	"github.com/z-chenhao/collie/internal/observability"
	"net/http/httptest"
	"strings"
	"testing"
)

type workerReader struct {
	calls   int
	running bool
	err     error
}

func (r *workerReader) WorkspaceWorkers(_ context.Context, running bool) (observability.WorkerActivityList, error) {
	r.calls++
	r.running = running
	return observability.WorkerActivityList{Items: []observability.WorkerActivity{{ID: "run", Title: "Research", Status: "running"}}}, r.err
}
func TestWorkerViewRequiresCurrentBindingAndReturnsOnlyActivity(t *testing.T) {
	core := &updatesCore{allowed: true}
	reader := &workerReader{}
	s := &Server{Core: core, Secrets: fakeSecrets{}, Workers: reader}
	request := func(path string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8080"+path, nil)
		if auth {
			r.Header.Set("Authorization", "Bearer private-token-at-least-24-bytes")
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w := request("/api/workers", false); w.Code != 401 || reader.calls != 0 {
		t.Fatal(w.Code)
	}
	w := request("/api/workers/current", true)
	if w.Code != 200 || !reader.running || w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, secret := range []string{"input_preview", "output_preview", "metrics", "model"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("unexpected payload", secret)
		}
	}
	if w = request("/api/workers", true); w.Code != 200 || reader.running {
		t.Fatal(w.Code)
	}
	if w = request("/api/workers?agent=foreign", true); w.Code != 400 {
		t.Fatal(w.Code)
	}
	reader.err = errors.New("PRIVATE_DATABASE")
	if w = request("/api/workers", true); w.Code != 503 || strings.Contains(w.Body.String(), "PRIVATE_DATABASE") {
		t.Fatal(w.Code, w.Body.String())
	}
	core.allowed = false
	before := reader.calls
	if w = request("/api/workers", true); w.Code != 403 || reader.calls != before {
		t.Fatal("revoked binding read workers", w.Code)
	}
}
func TestChatOwnsComputerAndWorkerWhileTasksRemainIndependent(t *testing.T) {
	s := &Server{Core: &updatesCore{allowed: true, status: "idle"}, Secrets: fakeSecrets{}, Tasks: workspaceReader{}}
	for _, path := range []string{"/", "/tasks", "/tasks/task:visible"} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8080"+path, nil)
		r.Header.Set("Authorization", "Bearer private-token-at-least-24-bytes")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(path, w.Code)
		}
		body := w.Body.String()
		for _, marker := range []string{`id="computer-panel"`, `id="worker-panel"`, `class="chat-identity"`} {
			if strings.Contains(body, marker) != (path == "/") {
				t.Fatal(path, marker)
			}
		}
	}
}
