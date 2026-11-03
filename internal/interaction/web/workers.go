package web

import (
	"context"
	"github.com/z-chenhao/collie/internal/observability"
	"net/http"
	"time"
)

type WorkerReader interface {
	WorkspaceWorkers(context.Context, bool) (observability.WorkerActivityList, error)
}

func (s *Server) workerView(w http.ResponseWriter, r *http.Request, runningOnly bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	if len(r.URL.Query()) != 0 {
		fail(w, 400)
		return
	}
	if s.Workers == nil {
		fail(w, 503)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	result, err := s.Workers.WorkspaceWorkers(ctx, runningOnly)
	if err != nil {
		fail(w, 503)
		return
	}
	jsonResult(w, result)
}
