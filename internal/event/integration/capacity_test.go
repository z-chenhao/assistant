package integration_test

import (
	"context"
	"testing"

	nat "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	eventnats "github.com/z-chenhao/erii/internal/event/nats"
	"github.com/z-chenhao/erii/internal/event/testbroker"
)

func TestNATSReopensWithinReservedBrokerCapacity(t *testing.T) {
	// The fixture's 2 GiB quota holds both streams, but not a second 1 GiB
	// event reservation. Reopening must validate and reuse the existing stream.
	broker := testbroker.New(t)
	bus := connect(t, broker)
	e := testEvent(t)
	checkErr(t, bus.Publish(context.Background(), e))
	checkErr(t, bus.Close())
	bus = connect(t, broker)
	h, err := bus.Health(context.Background())
	checkErr(t, err)
	if h.Events != 1 || h.MaxBytes != 1<<30 {
		t.Fatal("reopening changed retention or retained events", h)
	}
	ack(t, receive(t, subscribe(t, bus, "quota-reopen")))
}

func TestNATSReopenRejectsAdditionalConfigurationDrift(t *testing.T) {
	for name, mutate := range map[string]func(*jetstream.StreamConfig){
		"no-ack":            func(c *jetstream.StreamConfig) { c.NoAck = true },
		"per-subject-limit": func(c *jetstream.StreamConfig) { c.MaxMsgsPerSubject = 1 },
		"direct-access":     func(c *jetstream.StreamConfig) { c.AllowDirect = true },
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			broker := testbroker.New(t)
			bus := connect(t, broker)
			checkErr(t, bus.Close())
			conn, err := nat.Connect(broker.URL())
			checkErr(t, err)
			defer conn.Close()
			js, err := jetstream.New(conn)
			checkErr(t, err)
			stream, err := js.Stream(ctx, "test_events_v1")
			checkErr(t, err)
			info, err := stream.Info(ctx)
			checkErr(t, err)
			mutate(&info.Config)
			_, err = js.UpdateStream(ctx, info.Config)
			checkErr(t, err)
			if reopened, err := eventnats.Open(ctx, options(broker.URL())); err == nil {
				reopened.Close()
				t.Fatal("accepted drifted stream configuration")
			}
		})
	}
}
