// Package fake supplies a deterministic EventBus. Redelivery is explicit: an
// unacknowledged event remains the next delivery. There is no wall-clock retry.
package fake

import (
	"context"
	"errors"
	"sync"

	"github.com/z-chenhao/erii/internal/event"
)

type Bus struct {
	mu                           sync.Mutex
	events                       []event.CloudEvent
	offsets                      map[string]int
	changed                      chan struct{}
	publishFailures, ackFailures int
	closed                       bool
}

func New() *Bus { return &Bus{offsets: make(map[string]int), changed: make(chan struct{})} }

var _ event.EventBus = (*Bus)(nil)

func (b *Bus) FailNextPublish() { b.mu.Lock(); defer b.mu.Unlock(); b.publishFailures++ }
func (b *Bus) FailNextAck()     { b.mu.Lock(); defer b.mu.Unlock(); b.ackFailures++ }
func (b *Bus) Snapshot() []event.CloudEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make([]event.CloudEvent, len(b.events))
	for i, e := range b.events {
		result[i] = e.Clone()
	}
	return result
}
func (b *Bus) Publish(ctx context.Context, e event.CloudEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := event.Validate(e); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return errors.New("fake bus closed")
	}
	if b.publishFailures > 0 {
		b.publishFailures--
		return event.ErrUnavailable
	}
	b.events = append(b.events, e.Clone())
	close(b.changed)
	b.changed = make(chan struct{})
	return nil
}
func (b *Bus) Subscribe(ctx context.Context, consumer string) (event.Subscription, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if consumer == "" {
		return nil, errors.New("consumer required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, errors.New("fake bus closed")
	}
	return &subscription{bus: b, consumer: consumer, done: make(chan struct{})}, nil
}
func (b *Bus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.closed = true
		close(b.changed)
	}
	return nil
}

type subscription struct {
	bus      *Bus
	consumer string
	done     chan struct{}
	once     sync.Once
}

func (s *subscription) Close() error { s.once.Do(func() { close(s.done) }); return nil }
func (s *subscription) Next(ctx context.Context) (event.Delivery, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		select {
		case <-s.done:
			return nil, errors.New("subscription closed")
		default:
		}
		s.bus.mu.Lock()
		if s.bus.closed {
			s.bus.mu.Unlock()
			return nil, errors.New("fake bus closed")
		}
		i := s.bus.offsets[s.consumer]
		if i < len(s.bus.events) {
			e := s.bus.events[i].Clone()
			s.bus.mu.Unlock()
			return &delivery{sub: s, index: i, event: e}, nil
		}
		changed := s.bus.changed
		s.bus.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.done:
			return nil, errors.New("subscription closed")
		case <-changed:
		}
	}
}

type delivery struct {
	sub   *subscription
	index int
	event event.CloudEvent
}

func (d *delivery) Event() event.CloudEvent { return d.event.Clone() }
func (d *delivery) Ack(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b := d.sub.bus
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ackFailures > 0 {
		b.ackFailures--
		return errors.New("fake ack lost")
	}
	if b.offsets[d.sub.consumer] == d.index {
		b.offsets[d.sub.consumer]++
	}
	return nil
}
