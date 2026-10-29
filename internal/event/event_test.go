package event_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/z-chenhao/collie/internal/event"
	"github.com/z-chenhao/collie/internal/event/fake"
)

func TestCloudEventsProfile(t *testing.T) {
	e, err := event.New("/collie/test", "ai.collie.test.v1", "aggregate/with . * >", map[string]any{"version": 1})
	if err != nil {
		t.Fatal(err)
	}
	if id, err := uuid.Parse(e.ID()); err != nil || id.Version() != 7 {
		t.Fatal("host event ID must be UUIDv7")
	}
	e.SetExtension("traceparent", "00-test")
	data, err := event.Encode(e)
	if err != nil {
		t.Fatal(err)
	}
	again, err := event.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID() != e.ID() || again.Extensions()["traceparent"] != "00-test" {
		t.Fatal("structured roundtrip lost attributes")
	}
	for _, raw := range []string{`{}`, `{"specversion":"0.3"}`, `not json`, `{"specversion":"1.0","id":"x","source":"/test","type":"t"}`} {
		if _, err := event.Decode([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for name, edit := range map[string]func(*event.CloudEvent){
		"id":           func(e *event.CloudEvent) { e.SetID("") },
		"subject":      func(e *event.CloudEvent) { e.SetSubject("") },
		"content type": func(e *event.CloudEvent) { e.SetDataContentType("text/plain") },
		"non UTC time": func(e *event.CloudEvent) { e.SetTime(time.Now().In(time.FixedZone("offset", 8*3600))) },
	} {
		t.Run(name, func(t *testing.T) {
			bad := e.Clone()
			edit(&bad)
			if _, err := event.Encode(bad); err == nil {
				t.Fatal("accepted invalid event")
			}
		})
	}
}

type emptyOutbox struct {
	cancel func()
	err    error
}

func (o emptyOutbox) Pending(context.Context, int) ([]event.CloudEvent, error) {
	if o.cancel != nil {
		o.cancel()
	}
	return nil, o.err
}
func (emptyOutbox) RecordAttempt(context.Context, string) error { return nil }
func (emptyOutbox) MarkPublished(context.Context, string) error { return nil }

type trackedBus struct {
	event.EventBus
	closes int
}

func (b *trackedBus) Close() error { b.closes++; return nil }

func TestRelayRetriesTransientFailureAndClosesOnCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	b := &trackedBus{EventBus: fake.New()}
	calls := 0
	err := event.Relay(ctx, emptyOutbox{cancel: cancel}, func(context.Context) (event.EventBus, error) {
		calls++
		if calls == 1 {
			return nil, event.ErrUnavailable
		}
		return b, nil
	}, time.Millisecond, logger)
	if err != nil || calls != 2 || b.closes != 1 {
		t.Fatalf("err=%v connects=%d closes=%d", err, calls, b.closes)
	}
}

func TestRelayStopsOnPermanentFailure(t *testing.T) {
	for _, where := range []string{"connect", "outbox"} {
		t.Run(where, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			permanent := errors.New("permanent fault")
			calls := 0
			b := &trackedBus{EventBus: fake.New()}
			outbox := emptyOutbox{}
			if where == "outbox" {
				outbox.err = permanent
			}
			err := event.Relay(ctx, outbox, func(context.Context) (event.EventBus, error) {
				calls++
				if where == "connect" {
					return nil, permanent
				}
				return b, nil
			}, time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if !errors.Is(err, permanent) || calls != 1 {
				t.Fatalf("permanent fault retried: %v, %d", err, calls)
			}
			if where == "outbox" && b.closes != 1 {
				t.Fatal("broker leaked on permanent storage fault")
			}
		})
	}
}
