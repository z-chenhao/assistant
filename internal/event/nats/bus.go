// Package nats is a thin JetStream adapter for the event.EventBus boundary.
package nats

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"time"

	nat "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/z-chenhao/erii/internal/event"
)

type Options struct {
	URL           string        `yaml:"url"`
	StreamPrefix  string        `yaml:"stream_prefix"`
	Timeout       time.Duration `yaml:"timeout"`
	AckWait       time.Duration `yaml:"ack_wait"`
	RetryInterval time.Duration `yaml:"retry_interval"`
}

var namePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func (o Options) Validate() error {
	u, err := url.Parse(o.URL)
	if err != nil || u == nil || (u.Scheme != "nats" && u.Scheme != "tls") || u.Hostname() == "" || u.Port() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return errors.New("nats.url must be a nats:// or tls:// host:port without credentials, path, query, or fragment")
	}
	if !namePattern.MatchString(o.StreamPrefix) {
		return errors.New("nats.stream_prefix must contain 1-64 letters, digits, underscores, or hyphens")
	}
	if o.Timeout <= 0 || o.Timeout > time.Minute || o.AckWait <= 0 || o.RetryInterval <= 0 {
		return errors.New("NATS durations must be positive; timeout must not exceed one minute")
	}
	return nil
}

type Bus struct {
	conn             *nat.Conn
	js               jetstream.JetStream
	options          Options
	stream, subjects string
}

var _ event.EventBus = (*Bus)(nil)

func Open(ctx context.Context, options Options) (*Bus, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	timeout := options.Timeout
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < timeout {
		timeout = time.Until(deadline)
	}
	// Disable reconnect/buffering: the durable SQLite relay owns retry decisions.
	conn, err := nat.Connect(options.URL, nat.Timeout(timeout), nat.NoReconnect(), nat.NoEcho())
	if err != nil {
		return nil, brokerError(err, "connect event broker")
	}
	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, errors.New("initialize JetStream")
	}
	b := &Bus{conn: conn, js: js, options: options, stream: options.StreamPrefix + "_events_v1", subjects: options.StreamPrefix + ".v1.events"}
	ctx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()
	// Create validates existing configuration rather than silently mutating it.
	_, err = js.CreateStream(ctx, jetstream.StreamConfig{
		Name: b.stream, Subjects: []string{b.subjects + ".>"}, Storage: jetstream.FileStorage,
		Retention: jetstream.LimitsPolicy, Discard: jetstream.DiscardNew,
	})
	if err != nil {
		conn.Close()
		return nil, brokerError(err, "create or validate event stream")
	}
	return b, nil
}

func (b *Bus) Publish(ctx context.Context, e event.CloudEvent) error {
	data, err := event.Encode(e)
	if err != nil {
		return err
	}
	key := sha256.Sum256([]byte(e.Subject()))
	msg := nat.NewMsg(b.subjects + "." + hex.EncodeToString(key[:]))
	msg.Header.Set("Content-Type", event.ContentType)
	msg.Data = data
	// No broker deduplication window is used as a correctness dependency.
	ctx, cancel := context.WithTimeout(ctx, b.options.Timeout)
	defer cancel()
	if _, err := b.js.PublishMsg(ctx, msg); err != nil {
		return brokerError(err, "publish event to JetStream")
	}
	return nil
}

func (b *Bus) Subscribe(ctx context.Context, consumer string) (event.Subscription, error) {
	if !namePattern.MatchString(consumer) {
		return nil, errors.New("invalid durable consumer name")
	}
	ctx, cancel := context.WithTimeout(ctx, b.options.Timeout)
	defer cancel()
	c, err := b.js.CreateConsumer(ctx, b.stream, jetstream.ConsumerConfig{
		Durable: consumer, AckPolicy: jetstream.AckExplicitPolicy, AckWait: b.options.AckWait,
		DeliverPolicy: jetstream.DeliverAllPolicy, FilterSubject: b.subjects + ".>",
		MaxDeliver: -1, MaxAckPending: 1,
	})
	if err != nil {
		return nil, brokerError(err, "create or validate durable consumer")
	}
	messages, err := c.Messages(jetstream.PullMaxMessages(1))
	if err != nil {
		return nil, errors.New("open event subscription")
	}
	return &subscription{messages: messages, timeout: b.options.Timeout}, nil
}

func (b *Bus) Close() error { b.conn.Close(); return nil }

type subscription struct {
	messages jetstream.MessagesContext
	timeout  time.Duration
}

func (s *subscription) Close() error { s.messages.Stop(); return nil }
func (s *subscription) Next(ctx context.Context) (event.Delivery, error) {
	msg, err := s.messages.Next(jetstream.NextContext(ctx))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, brokerError(err, "receive event")
	}
	if msg.Headers().Get("Content-Type") != event.ContentType {
		return nil, errors.New("event message must use CloudEvents structured JSON")
	}
	e, err := event.Decode(msg.Data())
	if err != nil {
		return nil, err
	}
	return &delivery{message: msg, event: e, timeout: s.timeout}, nil
}

type delivery struct {
	message jetstream.Msg
	event   event.CloudEvent
	timeout time.Duration
}

func (d *delivery) Event() event.CloudEvent { return d.event.Clone() }
func (d *delivery) Ack(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	if err := d.message.DoubleAck(ctx); err != nil {
		return brokerError(err, "acknowledge event")
	}
	return nil
}

// Preserve only safe classification, never provider errors/URLs/credentials.
func brokerError(err error, action string) error {
	var network *net.OpError
	var api *jetstream.APIError
	if errors.As(err, &network) ||
		errors.Is(err, nat.ErrNoServers) ||
		errors.Is(err, nat.ErrConnectionClosed) ||
		errors.Is(err, nat.ErrDisconnected) ||
		errors.Is(err, nat.ErrTimeout) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, nat.ErrNoResponders) ||
		errors.Is(err, jetstream.ErrNoStreamResponse) ||
		(errors.As(err, &api) && api.Code == 503) {
		return fmt.Errorf("%s: %w", action, event.ErrUnavailable)
	}
	return errors.New(action)
}
