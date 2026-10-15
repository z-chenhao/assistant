package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	agentsqlite "github.com/z-chenhao/erii/internal/agent/sqlite"
	"testing"
	"time"

	nat "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/z-chenhao/erii/internal/event"
	"github.com/z-chenhao/erii/internal/event/fake"
	eventnats "github.com/z-chenhao/erii/internal/event/nats"
	"github.com/z-chenhao/erii/internal/event/testbroker"
)

func options(url string) eventnats.Options {
	return eventnats.Options{URL: url, StreamPrefix: "test", Timeout: time.Second, AckWait: 100 * time.Millisecond, RetryInterval: 10 * time.Millisecond}
}
func connect(t *testing.T, b *testbroker.Broker) *eventnats.Bus {
	t.Helper()
	bus, err := eventnats.Open(context.Background(), options(b.URL()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bus.Close() })
	return bus
}
func testEvent(t *testing.T) event.CloudEvent {
	t.Helper()
	e, err := event.New("/erii/test", "ai.erii.test.v1", "aggregate/with . * >", map[string]int{"version": 2})
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func receive(t *testing.T, s event.Subscription) event.Delivery {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d, err := s.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func subscribe(t *testing.T, bus event.EventBus, name string) event.Subscription {
	t.Helper()
	s, err := bus.Subscribe(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func ack(t *testing.T, d event.Delivery) {
	t.Helper()
	if err := d.Ack(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestEventBusContract(t *testing.T) {
	for _, kind := range []string{"fake", "nats"} {
		t.Run(kind, func(t *testing.T) {
			var bus event.EventBus = fake.New()
			if kind == "nats" {
				bus = connect(t, testbroker.New(t))
			}
			e := testEvent(t)
			e.SetExtension("traceparent", "00-preserved")
			for range 2 {
				if err := bus.Publish(context.Background(), e); err != nil {
					t.Fatal(err)
				}
			}
			sub := subscribe(t, bus, "contract")
			first := receive(t, sub)
			data, err := event.Encode(first.Event())
			if err != nil {
				t.Fatal(err)
			}
			var raw map[string]any
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatal(err)
			}
			if raw["specversion"] != "1.0" || raw["id"] != e.ID() || raw["traceparent"] != "00-preserved" {
				t.Fatalf("envelope: %s", data)
			}
			// Close/reopen without ack: the durable position must not advance.
			if err := sub.Close(); err != nil {
				t.Fatal(err)
			}
			sub = subscribe(t, bus, "contract")
			redelivered := receive(t, sub)
			if redelivered.Event().ID() != e.ID() {
				t.Fatal("unacked event lost")
			}
			ack(t, redelivered)
			second := receive(t, sub)
			if second.Event().ID() != e.ID() {
				t.Fatal("duplicate ID changed")
			}
			ack(t, second)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if _, err := sub.Next(ctx); err == nil {
				t.Fatal("acked event redelivered")
			}
			other := subscribe(t, bus, "independent")
			ack(t, receive(t, other))
			invalid := e.Clone()
			invalid.SetSubject("")
			if err := bus.Publish(context.Background(), invalid); err == nil {
				t.Fatal("invalid CloudEvent published")
			}
			checkErr(t, bus.Close())
			if err := bus.Publish(context.Background(), e); err == nil {
				t.Fatal("closed bus accepted publication")
			}
		})
	}
}

func TestNATSWireFormatAndRejectNonCloudEvents(t *testing.T) {
	broker := testbroker.New(t)
	bus := connect(t, broker)
	ctx := context.Background()
	e := testEvent(t)
	if err := bus.Publish(ctx, e); err != nil {
		t.Fatal(err)
	}
	conn, err := nat.Connect(broker.URL())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	js, err := jetstream.New(conn)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := js.Stream(ctx, "test_events_v1")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := stream.GetMsg(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if raw.Header.Get("Content-Type") != event.ContentType {
		t.Fatal("incorrect content type")
	}
	decoded, err := event.Decode(raw.Data)
	if err != nil || decoded.ID() != e.ID() {
		t.Fatalf("wire: %s %v", raw.Data, err)
	}
	sub := subscribe(t, bus, "wire")
	ack(t, receive(t, sub))
	if _, err := js.Publish(ctx, "test.v1.events.raw", []byte(`{"arbitrary":"json"}`)); err != nil {
		t.Fatal(err)
	}
	wait, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := sub.Next(wait); err == nil {
		t.Fatal("non-CloudEvent delivered to domain")
	}
	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Config.Storage != jetstream.FileStorage || info.Config.Discard != jetstream.DiscardNew {
		t.Fatal("stream durability configuration drift")
	}
}

func TestNATSBrokerRestartPreservesAcceptedUnackedEvent(t *testing.T) {
	broker := testbroker.New(t)
	bus := connect(t, broker)
	e := testEvent(t)
	if err := bus.Publish(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	sub := subscribe(t, bus, "restart")
	if receive(t, sub).Event().ID() != e.ID() {
		t.Fatal("wrong event")
	}
	_ = sub.Close()
	_ = bus.Close()
	broker.Stop()
	broker.Start(t)
	bus = connect(t, broker)
	sub = subscribe(t, bus, "restart")
	d := receive(t, sub)
	if d.Event().ID() != e.ID() {
		t.Fatal("broker restart lost accepted event")
	}
	ack(t, d)
}

func TestNATSRejectsIncompatibleStreamAndConsumer(t *testing.T) {
	broker := testbroker.New(t)
	ctx := context.Background()
	conn, err := nat.Connect(broker.URL())
	checkErr(t, err)
	defer conn.Close()
	js, err := jetstream.New(conn)
	checkErr(t, err)
	_, err = js.CreateStream(ctx, jetstream.StreamConfig{Name: "test_events_v1", Subjects: []string{"test.v1.events.>"}, Storage: jetstream.MemoryStorage})
	checkErr(t, err)
	if bus, err := eventnats.Open(ctx, options(broker.URL())); err == nil {
		_ = bus.Close()
		t.Fatal("incompatible stream accepted")
	}
	checkErr(t, js.DeleteStream(ctx, "test_events_v1"))
	bus := connect(t, broker)
	_, err = js.CreateConsumer(ctx, "test_events_v1", jetstream.ConsumerConfig{Durable: "incompatible", AckPolicy: jetstream.AckNonePolicy})
	checkErr(t, err)
	if sub, err := bus.Subscribe(ctx, "incompatible"); err == nil {
		_ = sub.Close()
		t.Fatal("incompatible consumer accepted")
	}
}

func TestPublisherRetainsPendingIntentDuringRealBrokerOutage(t *testing.T) {
	ctx := context.Background()
	broker := testbroker.New(t)
	bus := connect(t, broker)
	store, err := agentsqlite.Open(ctx, t.TempDir())
	checkErr(t, err)
	defer store.Close()
	broker.Stop()
	if n, err := (event.Publisher{Outbox: store.Events(), Bus: bus}).Flush(ctx, 100); n != 0 || !errors.Is(err, event.ErrUnavailable) {
		t.Fatalf("outage not classified as transient: %d %v", n, err)
	}
	if stateCount(t, store, "SELECT count(*) FROM event_outbox WHERE published_at IS NULL") != 1 {
		t.Fatal("broker outage lost event intent")
	}
	broker.Start(t)
	bus = connect(t, broker)
	if n, err := (event.Publisher{Outbox: store.Events(), Bus: bus}).Flush(ctx, 100); n != 1 || err != nil {
		t.Fatalf("recovery: %d %v", n, err)
	}
	if stateCount(t, store, "SELECT attempt_count FROM event_outbox") != 2 {
		t.Fatal("attempts not persisted across outage")
	}
}
