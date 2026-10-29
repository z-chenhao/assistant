package startup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	eventnats "github.com/z-chenhao/collie/internal/event/nats"
	"github.com/z-chenhao/collie/internal/maintenance"
)

func TestLocalBrokerRestartOwnershipAndMaintenance(t *testing.T) {
	root := t.TempDir()
	b, err := OpenLocalBroker(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	bus, err := eventnats.Open(context.Background(), eventnats.Options{URL: b.URL(), StreamPrefix: "startup", Timeout: 2 * time.Second, AckWait: 30 * time.Second, RetryInterval: time.Second})
	if err != nil {
		t.Fatal("managed broker cannot host the production event stream and DLQ", err)
	}
	bus.Close()
	if duplicate, err := OpenLocalBroker(context.Background(), root); err == nil {
		duplicate.Close()
		t.Fatal("second foreground instance acquired broker storage")
	}
	if lock, err := maintenance.Lock(root, true); !errors.Is(err, maintenance.ErrBusy) {
		if lock != nil {
			lock.Close()
		}
		t.Fatal("backup could race broker lifecycle", err)
	}
	connect := func(b *LocalBroker) (*nats.Conn, nats.JetStreamContext) {
		t.Helper()
		nc, err := nats.Connect(b.URL())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(nc.Close)
		js, err := nc.JetStream()
		if err != nil {
			t.Fatal(err)
		}
		return nc, js
	}
	nc, js := connect(b)
	if _, err = js.AddStream(&nats.StreamConfig{Name: "STARTUP", Subjects: []string{"startup"}, Storage: nats.FileStorage}); err != nil {
		t.Fatal(err)
	}
	if _, err = js.Publish("startup", []byte("persisted notification")); err != nil {
		t.Fatal(err)
	}
	nc.Close()
	b.Close()
	b, err = OpenLocalBroker(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	nc, js = connect(b)
	message, err := js.GetMsg("STARTUP", 1)
	if err != nil {
		t.Fatal("restart lost JetStream notification", err)
	}
	if string(message.Data) != "persisted notification" {
		t.Fatal("restart lost JetStream notification", message, err)
	}
	nc.Close()
	b.Close()
	lock, err := maintenance.Lock(root, true)
	if err != nil {
		t.Fatal("shutdown leaked maintenance lock", err)
	}
	lock.Close()
}

func TestCancelledStartupReleasesOwnership(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := t.TempDir()
	if b, err := OpenLocalBroker(ctx, root); !errors.Is(err, context.Canceled) {
		if b != nil {
			b.Close()
		}
		t.Fatal("cancelled startup accepted", err)
	}
	b, err := OpenLocalBroker(context.Background(), root)
	if err != nil {
		t.Fatal("cancelled startup leaked ownership", err)
	}
	b.Close()
}
