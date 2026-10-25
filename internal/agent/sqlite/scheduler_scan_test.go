package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/z-chenhao/erii/internal/agent"
)

func TestSchedulerScansReadFreshStateAcrossConnectionReplacement(t *testing.T) {
	s, r, a, _ := taskFixture(t)
	ctx := context.Background()
	first := mustSubmit(t, a, "first", "hello")
	if id, err := r.nextScheduled(ctx); err != nil || id != first.RunID {
		t.Fatal(id, err)
	}
	claim, err := r.Claim(ctx, first.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Next(ctx); !errors.Is(err, agent.ErrBusy) {
		t.Fatal("owned Main must keep Drain alive", err)
	}
	// Retire the connection which compiled the scans. database/sql must
	// reprepare them on the replacement without freezing rows or parameters.
	s.db.SetMaxIdleConns(0)
	s.db.SetMaxIdleConns(1)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := r.nextScheduled(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled scan ignored its context", err)
	}
	if err := r.Release(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if id, err := r.Next(ctx); err != nil || id != first.RunID {
		t.Fatal("released Main was not observed", id, err)
	}
	mustDrain(t, a)
	if _, err := r.Next(ctx); !errors.Is(err, agent.ErrNotFound) {
		t.Fatal("terminal Main kept Drain alive", err)
	}
	second := mustSubmit(t, a, "second", "hello again")
	if id, err := r.Next(ctx); err != nil || id != second.RunID || id == first.RunID {
		t.Fatal("new Main was not observed", id, err)
	}
	mustDrain(t, a)
}
