// Package delivery owns channel-neutral delivery policy. Semantic history is
// immutable; adapters own presentation and authoritative delivery receipts.
package delivery

import (
	"context"
	"errors"
	"time"

	"github.com/z-chenhao/erii/internal/conversation"
)

var (
	ErrStorage  = errors.New("delivery storage unavailable")
	ErrConflict = errors.New("delivery identity or ownership conflict")
	ErrEmpty    = errors.New("no delivery ready")
)

type Request struct {
	DeliveryID     string `json:"delivery_id"`
	MessageID      string `json:"message_id"`
	ConversationID string `json:"conversation_id"`
	Channel        string `json:"channel"`
	DestinationRef string `json:"destination_ref"`
	Intent         string `json:"intent"`
	Status         string `json:"status"`
	IdempotencyKey string `json:"idempotency_key"`
	AttemptCount   int    `json:"attempt_count"`
	LastError      string `json:"last_error,omitempty"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
	Version        int64  `json:"version"`
}

func (r Request) Validate() error {
	if r.DeliveryID == "" || r.MessageID == "" || r.ConversationID == "" || r.Channel == "" || r.DestinationRef == "" || r.IdempotencyKey != r.DeliveryID || r.CreatedAt <= 0 {
		return ErrConflict
	}
	switch r.Intent {
	case "completion", "failure", "needs_input", "approval":
		return nil
	}
	return ErrConflict
}

type Envelope struct {
	Request Request              `json:"request"`
	Message conversation.Message `json:"message"`
}

// Compose is the current text/media fallback. Channel-specific UI composition
// can evolve behind this boundary without changing semantic messages.
func Compose(r Request, m conversation.Message) (Envelope, error) {
	if r.Validate() != nil || m.MessageID != r.MessageID || m.ConversationID != r.ConversationID || m.Role != "assistant" {
		return Envelope{}, ErrConflict
	}
	parts, err := conversation.Normalize(m.Content)
	if err != nil {
		return Envelope{}, ErrConflict
	}
	for _, p := range parts {
		if p.Type == "tool_call" {
			return Envelope{}, ErrConflict
		}
	}
	m.Content = parts
	return Envelope{Request: r, Message: m}, nil
}

type Outcome string

const (
	Delivered Outcome = "sent"
	// Retry proves this attempt had no external effect.
	Retry   Outcome = "retry"
	Failed  Outcome = "failed"
	Unknown Outcome = "unknown"
)

type Observation struct {
	Outcome Outcome
	Code    string
}

// Query must return authoritative evidence. Retry means definitively absent;
// lack of query support or ambiguous failure means Unknown. Provider errors
// never carry arbitrary response bodies into storage or user history.
type Adapter interface {
	// Idempotent guarantees deduplication for the frozen action/key. An
	// expired sender may still be alive, so absence alone cannot permit
	// another send through an adapter without this guarantee.
	Idempotent() bool
	Send(context.Context, Envelope) Observation
	Query(context.Context, Envelope) Observation
}
type Claim struct {
	Request Request
	Token   string
	Recover bool
}
type Store interface {
	Claim(context.Context, string, string, []string, time.Time, time.Duration) (Claim, error)
	Settle(context.Context, Claim, Observation, time.Time, time.Time) error
}
type History interface {
	List(context.Context, string) ([]conversation.Message, error)
}

type Policy struct {
	LeaseSeconds      int `yaml:"lease_seconds"`
	RetryMilliseconds int `yaml:"retry_milliseconds"`
	MaxAttempts       int `yaml:"max_attempts"`
}

func DefaultPolicy() Policy { return Policy{LeaseSeconds: 5, RetryMilliseconds: 250, MaxAttempts: 5} }
func (p Policy) Validate() error {
	if p.LeaseSeconds < 1 || p.LeaseSeconds > 300 || p.RetryMilliseconds < 1 || p.RetryMilliseconds > 60000 || p.MaxAttempts < 1 || p.MaxAttempts > 20 {
		return ErrConflict
	}
	return nil
}

type Service struct {
	Store                Store
	History              History
	Adapter              Adapter
	Channel, Destination string
	Policy               Policy
	Boundary             func(string)
}

func (s Service) boundary(name string) {
	if s.Boundary != nil {
		s.Boundary(name)
	}
}

// Drain scans authoritative rows, independent of queue delivery or model
// readiness. Each row is handled once per scan; retry wake times are persisted.
func (s Service) Drain(ctx context.Context) error {
	if s.Policy.Validate() != nil || s.Adapter == nil {
		return ErrConflict
	}
	seen := []string{}
	for ctx.Err() == nil {
		c, err := s.Store.Claim(ctx, s.Channel, s.Destination, seen, time.Now(), time.Duration(s.Policy.LeaseSeconds)*time.Second)
		if errors.Is(err, ErrEmpty) {
			return nil
		}
		if err != nil {
			return err
		}
		seen = append(seen, c.Request.DeliveryID)
		messages, err := s.History.List(ctx, c.Request.ConversationID)
		if err != nil {
			return err
		}
		var message conversation.Message
		for _, m := range messages {
			if m.MessageID == c.Request.MessageID {
				message = m
				break
			}
		}
		e, err := Compose(c.Request, message)
		if err != nil {
			return err
		}
		observation := Observation{Outcome: Retry}
		if c.Recover {
			observation = s.Adapter.Query(ctx, e)
		}
		if c.Recover && observation.Outcome == Retry && !s.Adapter.Idempotent() {
			observation = Observation{Outcome: Unknown}
		}
		// An unknown row is query-only. A recovery scan cannot turn absence or
		// a query failure into permission to repeat an ambiguous side effect.
		if c.Request.Status == "unknown" && observation.Outcome == Retry {
			observation = Observation{Outcome: Unknown}
		}
		if observation.Outcome == Retry {
			if c.Request.AttemptCount > s.Policy.MaxAttempts {
				observation = Observation{Outcome: Failed, Code: "retry_exhausted"}
			} else {
				s.boundary("before_delivery_send")
				observation = s.Adapter.Send(ctx, e)
				s.boundary("after_delivery_send")
			}
		}
		if observation.Outcome == Retry && c.Request.AttemptCount >= s.Policy.MaxAttempts {
			observation = Observation{Outcome: Failed, Code: "retry_exhausted"}
		}
		now := time.Now()
		delay := time.Duration(s.Policy.RetryMilliseconds) * time.Millisecond * time.Duration(1<<min(c.Request.AttemptCount-1, 8))
		s.boundary("before_delivery_ack")
		if err := s.Store.Settle(ctx, c, observation, now, now.Add(delay)); err != nil {
			return err
		}
		s.boundary("after_delivery_ack")
	}
	return ctx.Err()
}
