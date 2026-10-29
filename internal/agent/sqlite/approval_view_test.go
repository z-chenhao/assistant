package sqlite

import (
	"context"
	"errors"
	"github.com/z-chenhao/erii/internal/tool"
	"testing"
	"time"
)

func TestWebApprovalViewDoesNotRecoverOrWakeExpiredWork(t *testing.T) {
	ctx := context.Background()
	calls := effectCall(`{"target":"Alex","text":"Review this action"}`)
	s, r, _, executor, exchange := admitPolicy(t, []tool.Definition{effectDef(t, "query_before_retry")}, calls)
	if _, err := executor.Execute(ctx, exchange, calls); !errors.Is(err, tool.ErrSuspended) {
		t.Fatal(err)
	}
	r.approvalClock = func() time.Time { return time.Now().Add(2 * time.Hour) }
	events := scalar(t, s, `SELECT count(*) FROM event_outbox`)
	views, err := r.ApprovalView(ctx)
	if err != nil || len(views) != 1 || views[0].Status != "expired" {
		t.Fatal(views, err)
	}
	if scalar(t, s, `SELECT count(*) FROM approval WHERE status='pending'`) != 1 || scalar(t, s, `SELECT count(*) FROM event_outbox`) != events || scalar(t, s, `SELECT sum(attempt_count) FROM tool_invocation`) != 0 {
		t.Fatal("browsing recovered or dispatched an action")
	}
	if _, err = r.Approvals(ctx); err != nil {
		t.Fatal(err)
	}
	if scalar(t, s, `SELECT count(*) FROM approval WHERE status='expired'`) != 1 {
		t.Fatal("CLI/runtime recovery behavior changed")
	}
}
