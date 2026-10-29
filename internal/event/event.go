// Package event owns the internal CloudEvents boundary and at-least-once relay.
// No NATS or SQL types cross the EventBus boundary.
package event

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	ce "github.com/cloudevents/sdk-go/v2/event"
	"github.com/google/uuid"
)

type CloudEvent = ce.Event

const ContentType = "application/cloudevents+json"

// ErrUnavailable marks a known transient broker failure, safe to retry.
var ErrUnavailable = errors.New("event broker unavailable")

// ErrPoison never contains the message body. Operators inspect the local DLQ.
var ErrPoison = errors.New("event quarantined or awaiting poison threshold")

type FailureOutbox interface {
	RecordFailure(context.Context, string) error
}

// New creates a host-owned, globally unique event. Subject is the aggregate key.
func New(source, typ, subject string, data any) (CloudEvent, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return CloudEvent{}, err
	}
	e := ce.New("1.0")
	e.SetID(id.String())
	e.SetSource(source)
	e.SetType(typ)
	e.SetSubject(subject)
	e.SetTime(time.Now().UTC())
	if err := e.SetData("application/json", data); err != nil {
		return CloudEvent{}, err
	}
	return e, Validate(e)
}

// Validate is Collie's CloudEvents 1.0 JSON profile: a subject and timestamp are
// required and payloads are JSON. The SDK preserves standard/extension fields.
func Validate(e CloudEvent) error {
	if err := e.Validate(); err != nil {
		return errors.New("invalid CloudEvent")
	}
	if e.SpecVersion() != "1.0" || e.Subject() == "" || e.Time().IsZero() || e.DataContentType() != "application/json" || !json.Valid(e.Data()) {
		return errors.New("CloudEvent requires version 1.0, subject, time, and JSON data")
	}
	_, offset := e.Time().Zone()
	if offset != 0 {
		return errors.New("CloudEvent time must be UTC")
	}
	return nil
}

func Encode(e CloudEvent) ([]byte, error) {
	if err := Validate(e); err != nil {
		return nil, err
	}
	return json.Marshal(e)
}

func Decode(data []byte) (CloudEvent, error) {
	var e CloudEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return e, errors.New("invalid CloudEvents structured JSON")
	}
	return e, Validate(e)
}

type Delivery interface {
	Event() CloudEvent
	// Ack is called only after the local Inbox/domain transaction commits.
	Ack(context.Context) error
}

type Subscription interface {
	Next(context.Context) (Delivery, error)
	Close() error
}

type EventBus interface {
	// Publish returns success only after durable broker acceptance.
	Publish(context.Context, CloudEvent) error
	Subscribe(context.Context, string) (Subscription, error)
	Close() error
}

// Outbox is used by Publisher. Domain writes enqueue through their SQL
// transaction; the relay never performs a domain write.
type Outbox interface {
	Pending(context.Context, int) ([]CloudEvent, error)
	RecordAttempt(context.Context, string) error
	MarkPublished(context.Context, string) error
}

type Publisher struct {
	Outbox Outbox
	Bus    EventBus
}

// Flush makes one bounded pass. A publish/mark ambiguity leaves the record
// pending. Repeating the pass may publish it again with the SAME event ID.
func (p Publisher) Flush(ctx context.Context, limit int) (int, error) {
	events, err := p.Outbox.Pending(ctx, limit)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, e := range events {
		if err := p.Outbox.RecordAttempt(ctx, e.ID()); err != nil {
			return count, err
		}
		if err := p.Bus.Publish(ctx, e); err != nil {
			if !errors.Is(err, ErrUnavailable) && ctx.Err() == nil {
				if failures, ok := p.Outbox.(FailureOutbox); ok {
					if e := failures.RecordFailure(ctx, e.ID()); e != nil {
						return count, e
					}
					continue
				}
			}
			return count, err
		}
		if err := p.Outbox.MarkPublished(ctx, e.ID()); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// Relay retries internal event publication, never external Tool side effects.
// Failed connections are replaced; each attempt and shutdown is context bounded.
func Relay(ctx context.Context, outbox Outbox, connect func(context.Context) (EventBus, error), interval time.Duration, logger *slog.Logger) error {
	if interval <= 0 {
		return errors.New("relay interval must be positive")
	}
	var bus EventBus
	defer func() {
		if bus != nil {
			_ = bus.Close()
		}
	}()
	for {
		if ctx.Err() != nil {
			return nil
		}
		var err error
		if bus == nil {
			bus, err = connect(ctx)
		}
		if err == nil {
			_, err = (Publisher{Outbox: outbox, Bus: bus}).Flush(ctx, 100)
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if !errors.Is(err, ErrUnavailable) {
				return err
			}
			logger.Warn("event relay attempt failed; durable outbox retained")
			if bus != nil {
				_ = bus.Close()
				bus = nil
			}
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
