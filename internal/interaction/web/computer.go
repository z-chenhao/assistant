package web

import (
	"context"
	"net/http"
	"time"

	"github.com/z-chenhao/collie/internal/computer"
)

type ComputerReader interface {
	ComputerStatus(context.Context) (computer.Status, error)
	ComputerFrame(context.Context) ([]byte, error)
	ComputerActivity(context.Context) ([]computer.Activity, error)
}

func (s *Server) computerView(w http.ResponseWriter, r *http.Request, frame bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	if s.Computer == nil {
		fail(w, 503)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if frame {
		body, err := s.Computer.ComputerFrame(ctx)
		if err != nil || len(body) == 0 || len(body) > 12<<20 {
			fail(w, 503)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
		return
	}
	status, err := s.Computer.ComputerStatus(ctx)
	if err != nil {
		fail(w, 503)
		return
	}
	activity, activityErr := s.Computer.ComputerActivity(ctx)
	if activity == nil {
		activity = []computer.Activity{}
	}
	jsonResult(w, struct {
		Phase             string              `json:"phase"`
		Activity          []computer.Activity `json:"activity"`
		ActivityAvailable bool                `json:"activity_available"`
	}{Phase: status.Phase, Activity: activity, ActivityAvailable: activityErr == nil})
}
