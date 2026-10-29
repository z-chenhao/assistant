package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	agentsqlite "github.com/z-chenhao/collie/internal/agent/sqlite"
	"github.com/z-chenhao/collie/internal/event"
	"github.com/z-chenhao/collie/internal/event/fake"
	eventsqlite "github.com/z-chenhao/collie/internal/event/sqlite"
)

var ctx = context.Background()

func open(t *testing.T, dir string) *agentsqlite.Store {
	t.Helper()
	s, err := agentsqlite.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}
func fixture(t *testing.T, s *agentsqlite.Store) {
	t.Helper()
	if err := s.Transaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TABLE effect (id INTEGER PRIMARY KEY, value INTEGER NOT NULL, version INTEGER NOT NULL, epoch INTEGER NOT NULL); INSERT INTO effect VALUES (1,0,1,7)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
func count(t *testing.T, s *agentsqlite.Store, query string) int {
	t.Helper()
	var n int
	if err := s.Transaction(ctx, func(tx *sql.Tx) error { return tx.QueryRow(query).Scan(&n) }); err != nil {
		t.Fatal(err)
	}
	return n
}
func newEvent(t *testing.T, version, epoch int) event.CloudEvent {
	t.Helper()
	e, err := event.New("/test", "ai.collie.test.v1", "aggregate", map[string]int{"version": version, "epoch": epoch})
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func increment(ctx context.Context, tx *sql.Tx, _ event.CloudEvent) error {
	_, err := tx.ExecContext(ctx, `UPDATE effect SET value=value+1 WHERE id=1`)
	return err
}
func next(t *testing.T, b event.EventBus, consumer string) event.Delivery {
	t.Helper()
	sub, err := b.Subscribe(ctx, consumer)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sub.Close() })
	d, err := sub.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDomainAndOutboxCommitOrRollback(t *testing.T) {
	s := open(t, t.TempDir())
	fixture(t, s)
	e := newEvent(t, 2, 7)
	fail := errors.New("before commit")
	write := func(tx *sql.Tx) error {
		if err := increment(ctx, tx, e); err != nil {
			return err
		}
		return eventsqlite.Enqueue(ctx, tx, e)
	}
	if err := s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := write(tx); err != nil {
			return err
		}
		return fail
	}); !errors.Is(err, fail) {
		t.Fatal(err)
	}
	if count(t, s, "SELECT value FROM effect") != 0 || count(t, s, "SELECT count(*) FROM event_outbox") != 1 {
		t.Fatal("partial domain/outbox commit")
	}
	if err := s.Transaction(ctx, write); err != nil {
		t.Fatal(err)
	}
	if err := s.Transaction(ctx, write); err == nil {
		t.Fatal("duplicate ID accepted")
	}
	drift := e.Clone()
	drift.SetSubject("changed")
	if err := s.Transaction(ctx, func(tx *sql.Tx) error { return eventsqlite.Enqueue(ctx, tx, drift) }); err == nil {
		t.Fatal("ID parameter drift accepted")
	}
	if count(t, s, "SELECT value FROM effect") != 1 || count(t, s, "SELECT count(*) FROM event_outbox") != 2 {
		t.Fatal("duplicate changed state")
	}
	if _, err := s.Events().Pending(ctx, 0); err == nil {
		t.Fatal("unbounded batch accepted")
	}
}

func TestInboxEffectRollbackAckFailureAndConsumerScope(t *testing.T) {
	s := open(t, t.TempDir())
	fixture(t, s)
	b := fake.New()
	e := newEvent(t, 2, 7)
	if err := b.Publish(ctx, e); err != nil {
		t.Fatal(err)
	}
	d := next(t, b, "worker")
	fail := errors.New("effect interrupted")
	if handled, err := s.Events().Handle(ctx, "worker", d, func(ctx context.Context, tx *sql.Tx, e event.CloudEvent) error {
		if err := increment(ctx, tx, e); err != nil {
			return err
		}
		return fail
	}); handled || !errors.Is(err, fail) {
		t.Fatalf("%v %v", handled, err)
	}
	if count(t, s, "SELECT value FROM effect") != 0 || count(t, s, "SELECT count(*) FROM event_inbox") != 0 {
		t.Fatal("effect failure partially committed")
	}
	b.FailNextAck()
	if handled, err := s.Events().Handle(ctx, "worker", d, increment); !handled || err == nil {
		t.Fatalf("ack fault: %v %v", handled, err)
	}
	if count(t, s, "SELECT value FROM effect") != 1 || count(t, s, "SELECT count(*) FROM event_inbox") != 1 {
		t.Fatal("ack failure undid committed effect")
	}
	if handled, err := s.Events().Handle(ctx, "worker", next(t, b, "worker"), increment); handled || err != nil {
		t.Fatalf("dedup: %v %v", handled, err)
	}
	if handled, err := s.Events().Handle(ctx, "other", next(t, b, "other"), increment); !handled || err != nil {
		t.Fatalf("consumer scope: %v %v", handled, err)
	}
	if count(t, s, "SELECT value FROM effect") != 2 {
		t.Fatal("consumer scope not independent")
	}
}

type inspectDelivery struct {
	e   event.CloudEvent
	ack func() error
}

func (d inspectDelivery) Event() event.CloudEvent   { return d.e.Clone() }
func (d inspectDelivery) Ack(context.Context) error { return d.ack() }
func TestAckObservesCommittedDomainAndInbox(t *testing.T) {
	s := open(t, t.TempDir())
	fixture(t, s)
	d := inspectDelivery{e: newEvent(t, 2, 7), ack: func() error {
		if count(t, s, "SELECT value FROM effect") != 1 || count(t, s, "SELECT count(*) FROM event_inbox") != 1 {
			t.Error("ack preceded commit")
		}
		return nil
	}}
	if handled, err := s.Events().Handle(ctx, "worker", d, increment); !handled || err != nil {
		t.Fatalf("%v %v", handled, err)
	}
}

func TestDuplicateLateAndOutOfOrderSnapshots(t *testing.T) {
	s := open(t, t.TempDir())
	fixture(t, s)
	b := fake.New()
	high := newEvent(t, 4, 7)
	low := newEvent(t, 2, 7)
	oldEpoch := newEvent(t, 99, 6)
	latest := newEvent(t, 5, 7)
	for _, e := range []event.CloudEvent{high, low, high, oldEpoch, latest, low} {
		if err := b.Publish(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	sub, err := b.Subscribe(ctx, "projection")
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	effect := func(ctx context.Context, tx *sql.Tx, e event.CloudEvent) error {
		var data struct {
			Version int `json:"version"`
			Epoch   int `json:"epoch"`
		}
		if err := e.DataAs(&data); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE effect SET value=?,version=? WHERE id=1 AND version < ? AND epoch=?`, data.Version, data.Version, data.Version, data.Epoch)
		return err
	}
	handled := 0
	for range 6 {
		d, err := sub.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		did, err := s.Events().Handle(ctx, "projection", d, effect)
		if err != nil {
			t.Fatal(err)
		}
		if did {
			handled++
		}
	}
	if handled != 4 || count(t, s, "SELECT value FROM effect") != 5 || count(t, s, "SELECT count(*) FROM event_inbox") != 4 {
		t.Fatal("duplicate/late/out-of-order state advanced incorrectly")
	}
}

func TestConcurrentInboxAcrossConnectionsCommitsOneEffect(t *testing.T) {
	dir := t.TempDir()
	first := open(t, dir)
	fixture(t, first)
	second := open(t, dir)
	var acks atomic.Int32
	d := inspectDelivery{e: newEvent(t, 2, 7), ack: func() error { acks.Add(1); return nil }}
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Go(func() {
			s := first
			if i%2 == 1 {
				s = second
			}
			if _, err := s.Events().Handle(ctx, "worker", d, increment); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if acks.Load() != 12 || count(t, first, "SELECT value FROM effect") != 1 || count(t, first, "SELECT count(*) FROM event_inbox") != 1 {
		t.Fatal("concurrent duplicate effect")
	}
}

type failMark struct{ event.Outbox }

func (f failMark) MarkPublished(context.Context, string) error {
	return errors.New("crash after publish")
}
func TestPublisherRetainsIntentAcrossOutageAndAmbiguousPublish(t *testing.T) {
	dir := t.TempDir()
	s, err := agentsqlite.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	b := fake.New()
	b.FailNextPublish()
	if n, err := (event.Publisher{Outbox: s.Events(), Bus: b}).Flush(ctx, 100); n != 0 || err == nil {
		t.Fatal("outage was marked published")
	}
	if n, err := (event.Publisher{Outbox: failMark{s.Events()}, Bus: b}).Flush(ctx, 100); n != 0 || err == nil {
		t.Fatal("mark fault ignored")
	}
	first := b.Snapshot()
	if len(first) != 1 {
		t.Fatal("publish not accepted")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = open(t, dir)
	if n, err := (event.Publisher{Outbox: s.Events(), Bus: b}).Flush(ctx, 100); n != 1 || err != nil {
		t.Fatalf("restart: %d %v", n, err)
	}
	events := b.Snapshot()
	if len(events) != 2 || events[0].ID() != events[1].ID() || count(t, s, "SELECT attempt_count FROM event_outbox") != 3 || count(t, s, "SELECT count(*) FROM event_outbox WHERE published_at IS NULL") != 0 {
		t.Fatal("restart lost intent or changed ID")
	}
}
