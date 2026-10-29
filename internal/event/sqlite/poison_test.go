package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"github.com/z-chenhao/collie/internal/event"
	"github.com/z-chenhao/collie/internal/event/fake"
	eventsqlite "github.com/z-chenhao/collie/internal/event/sqlite"
	"testing"
)

type rejectingBus struct {
	event.EventBus
	reject  string
	offline bool
}

func (b rejectingBus) Publish(ctx context.Context, e event.CloudEvent) error {
	if b.offline {
		return event.ErrUnavailable
	}
	if e.ID() == b.reject {
		return errors.New("PRIVATE_PROVIDER_ERROR")
	}
	return b.EventBus.Publish(ctx, e)
}
func TestPoisonOutboxCannotBlockOtherAggregatesOrExhaustOnOutage(t *testing.T) {
	s := open(t, t.TempDir())
	outbox := s.Events()
	outbox.MaxFailures = 2
	var alerts []string
	outbox.OnDead = func(code string) { alerts = append(alerts, code) }
	pending, e := outbox.Pending(ctx, 100)
	if e != nil {
		t.Fatal(e)
	}
	poison := pending[0].ID()
	for range 9 {
		_, e = (event.Publisher{Outbox: outbox, Bus: rejectingBus{offline: true}}).Flush(ctx, 100)
		if !errors.Is(e, event.ErrUnavailable) {
			t.Fatal(e)
		}
	}
	if n := count(t, s, "SELECT count(*) FROM event_outbox WHERE dead_at IS NOT NULL OR failure_count!=0"); n != 0 {
		t.Fatal("outage exhausted poison threshold")
	}
	bus := fake.New()
	defer bus.Close()
	if e = s.Transaction(ctx, func(tx *sql.Tx) error { good := newEvent(t, 1, 1); return eventsqlite.Enqueue(ctx, tx, good) }); e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if _, e = (event.Publisher{Outbox: outbox, Bus: rejectingBus{EventBus: bus, reject: poison}}).Flush(ctx, 100); e != nil {
			t.Fatal(e)
		}
	}
	if len(bus.Snapshot()) != 1 || count(t, s, "SELECT count(*) FROM event_outbox WHERE dead_at IS NOT NULL AND failure_count=2") != 1 {
		t.Fatal("healthy wake blocked or missing DLQ")
	}
	if len(alerts) != 1 || alerts[0] != "EVENT_OUTBOX_POISON" {
		t.Fatal("missing safe developer alert", alerts)
	}
	if e = outbox.Retry(ctx, poison, 10); e == nil {
		t.Fatal("stale retry accepted")
	}
	if e = outbox.Retry(ctx, poison, 11); e != nil {
		t.Fatal(e)
	}
	if _, e = (event.Publisher{Outbox: outbox, Bus: bus}).Flush(ctx, 100); e != nil {
		t.Fatal(e)
	}
	if len(bus.Snapshot()) != 2 {
		t.Fatal("requeue lost event")
	}
	if e = s.Transaction(ctx, func(tx *sql.Tx) error {
		_, e := tx.Exec("UPDATE event_outbox SET payload_json='{}',published_at=NULL WHERE event_id=?", poison)
		return e
	}); e != nil {
		t.Fatal(e)
	}
	if _, e = outbox.Pending(ctx, 100); e != nil {
		t.Fatal(e)
	}
	if count(t, s, "SELECT count(*) FROM event_outbox WHERE failure_code='INVALID_ENVELOPE'") != 1 {
		t.Fatal("invalid envelope not isolated")
	}
	if len(alerts) != 2 || alerts[1] != "EVENT_OUTBOX_INVALID" {
		t.Fatal("missing malformed-envelope alert", alerts)
	}
	if e = outbox.Retry(ctx, poison, 1); e == nil {
		t.Fatal("invalid envelope retried")
	}
}
