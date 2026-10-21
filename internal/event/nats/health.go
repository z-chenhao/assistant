package nats

import (
	"context"
	"github.com/nats-io/nats.go/jetstream"
)

type Health struct {
	Events        uint64 `json:"events"`
	Bytes         uint64 `json:"bytes"`
	Dead          uint64 `json:"dead"`
	Pending       uint64 `json:"pending"`
	AckPending    int    `json:"ack_pending"`
	Redelivered   int    `json:"redelivered"`
	MaxBytes      int64  `json:"max_bytes"`
	MaxAgeSeconds int64  `json:"max_age_seconds"`
}

func (b *Bus) Health(ctx context.Context) (Health, error) {
	var h Health
	ctx, cancel := context.WithTimeout(ctx, b.options.Timeout)
	defer cancel()
	stream, e := b.js.Stream(ctx, b.stream)
	if e != nil {
		return h, brokerError(e, "inspect event stream")
	}
	info, e := stream.Info(ctx)
	if e != nil {
		return h, brokerError(e, "inspect event stream")
	}
	h.Events = info.State.Msgs
	h.Bytes = info.State.Bytes
	h.MaxBytes = info.Config.MaxBytes
	h.MaxAgeSeconds = int64(info.Config.MaxAge.Seconds())
	dlq, e := b.js.Stream(ctx, b.options.StreamPrefix+"_dlq_v1")
	if e != nil {
		return h, brokerError(e, "inspect DLQ")
	}
	dead, e := dlq.Info(ctx)
	if e != nil {
		return h, brokerError(e, "inspect DLQ")
	}
	h.Dead = dead.State.Msgs
	consumer, e := b.js.Consumer(ctx, b.stream, "runtime_wake_v1")
	if e == jetstream.ErrConsumerNotFound {
		return h, nil
	}
	if e != nil {
		return h, brokerError(e, "inspect event consumer")
	}
	c, e := consumer.Info(ctx)
	if e != nil {
		return h, brokerError(e, "inspect event consumer")
	}
	h.Pending = c.NumPending
	h.AckPending = c.NumAckPending
	h.Redelivered = c.NumRedelivered
	return h, nil
}
