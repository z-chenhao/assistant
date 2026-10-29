package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/z-chenhao/collie/internal/agent"
)

// This bounded counterfactual keeps the database, rows, SQL, arguments and
// process identical. Only statement reuse differs. Labels permit CPU attribution
// with go test -race -cpuprofile; timings are observations, never a flaky gate
// or a claim that the archived Ubuntu deadline failure has been reproduced.
func TestSchedulerScanCompilationDiagnostic(t *testing.T) {
	s, r, a, _ := taskFixture(t)
	want := mustSubmit(t, a, "scan-diagnostic", "hello")
	ctx := context.Background()
	const pairs = 16
	var elapsed [2]time.Duration
	for i := 0; i < pairs; i++ {
		args := r.scheduleArgs()
		for j := 0; j < 2; j++ {
			mode := (i + j) % 2 // Alternate order to limit warmup/order bias.
			label := []string{"reused", "recompiled"}[mode]
			pprof.Do(ctx, pprof.Labels("scheduler_scan", label), func(ctx context.Context) {
				start := time.Now()
				var id string
				var active int
				var err error
				if mode == 0 {
					err = s.nextRun.QueryRowContext(ctx, args...).Scan(&id)
					if err == nil {
						err = s.activeRun.QueryRowContext(ctx, sql.Named("now", int64(1<<63-1))).Scan(&active)
					}
				} else {
					err = s.db.QueryRowContext(ctx, nextRunSQL, args...).Scan(&id)
					if err == nil {
						err = s.db.QueryRowContext(ctx, activeRunSQL, sql.Named("now", int64(1<<63-1))).Scan(&active)
					}
				}
				elapsed[mode] += time.Since(start)
				if err != nil || id != want.RunID || active != 1 {
					t.Fatalf("%s scan changed admission: id=%s active=%d err=%v", label, id, active, err)
				}
			})
		}
	}
	t.Logf("scheduler scan diagnostic: pairs=%d queries_per_mode=%d reused_wall=%s recompiled_wall=%s", pairs, pairs*2, elapsed[0], elapsed[1])
}

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
