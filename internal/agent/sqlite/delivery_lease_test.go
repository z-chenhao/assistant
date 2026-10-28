package sqlite

import (
	"context"
	"errors"
	"github.com/z-chenhao/erii/internal/conversation"
	"sync"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/delivery"
)

func TestDeliveryRenewRejectsExpiredStaleAndTakenOverClaims(t *testing.T) {
	s, service, _ := terminalFixture(t)
	ctx := context.Background()
	store := s.Deliveries()
	c, err := store.Claim(ctx, service.Channel, service.Destination, nil, time.Now(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	before := deliveryRows(t, s)[0]
	if err = store.Renew(ctx, c, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if after := deliveryRows(t, s)[0]; after != before {
		t.Fatal("renewal changed semantic state", before, after)
	}
	for _, field := range []string{"token", "version"} {
		stale := c
		if field == "token" {
			stale.Token = "old-owner"
		} else {
			stale.Request.Version--
		}
		if err = store.Renew(ctx, stale, time.Hour); !errors.Is(err, delivery.ErrClaimLost) {
			t.Fatal("renewed stale claim", field, err)
		}
	}
	if _, err = s.db.Exec(`UPDATE delivery_outbox SET claim_expires_at=0`); err != nil {
		t.Fatal(err)
	}
	if err = store.Renew(ctx, c, time.Hour); !errors.Is(err, delivery.ErrClaimLost) {
		t.Fatal("revived expired owner", err)
	}
	next, err := store.Claim(ctx, service.Channel, service.Destination, nil, time.Now(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Renew(ctx, c, time.Hour); !errors.Is(err, delivery.ErrClaimLost) {
		t.Fatal("old owner renewed after takeover", err)
	}
	if err = store.Settle(ctx, c, delivery.Observation{Outcome: delivery.Delivered}, time.Now(), time.Now()); !errors.Is(err, delivery.ErrClaimLost) {
		t.Fatal("late acknowledgement accepted", err)
	}
	if err = store.Renew(ctx, next, time.Second); err != nil {
		t.Fatal("current owner lost claim", err)
	}
}

type interruptedDeliveryClaim struct {
	delivery.Store
	afterClaim func()
}

func (s interruptedDeliveryClaim) Claim(ctx context.Context, channel, destination string, excluded []string, now time.Time, lease time.Duration) (delivery.Claim, error) {
	c, err := s.Store.Claim(ctx, channel, destination, excluded, now, lease)
	if err == nil {
		s.afterClaim()
	}
	return c, err
}

func TestDeliveryExpiredBeforeTransportDoesNotSend(t *testing.T) {
	s, service, adapter := terminalFixture(t)
	service.Store = interruptedDeliveryClaim{Store: s.Deliveries(), afterClaim: func() {
		// A scheduling pause can outlast the lease before the heartbeat starts.
		if _, err := s.db.Exec(`UPDATE delivery_outbox SET claim_expires_at=0`); err != nil {
			t.Fatal(err)
		}
	}}
	if err := service.Drain(context.Background()); err != nil {
		t.Fatal("expired claim stopped consumer", err)
	}
	items, err := adapter.Messages(context.Background(), service.Destination)
	if err != nil || len(items) != 0 {
		t.Fatal("expired owner started transport", items, err)
	}
}

type heldDelivery struct {
	delivery.Adapter
	query bool
	enter chan struct{}
	leave chan struct{}
}

func (a heldDelivery) hold(ctx context.Context) bool {
	close(a.enter)
	select {
	case <-a.leave:
		return true
	case <-ctx.Done():
		return false
	}
}
func (a heldDelivery) Send(ctx context.Context, e delivery.Envelope) delivery.Observation {
	if !a.query && !a.hold(ctx) {
		return delivery.Observation{Outcome: delivery.Unknown}
	}
	return a.Adapter.Send(ctx, e)
}
func (a heldDelivery) Query(ctx context.Context, e delivery.Envelope) delivery.Observation {
	if a.query && !a.hold(ctx) {
		return delivery.Observation{Outcome: delivery.Unknown}
	}
	return a.Adapter.Query(ctx, e)
}

func TestDeliverySlowSendAndQueryRetainClaim(t *testing.T) {
	for _, query := range []bool{false, true} {
		name := "send"
		if query {
			name = "query"
		}
		t.Run(name, func(t *testing.T) {
			s, service, base := terminalFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if query {
				c, err := s.Deliveries().Claim(ctx, service.Channel, service.Destination, nil, time.Now(), time.Second)
				if err != nil {
					t.Fatal(err)
				}
				messages, err := service.History.(conversation.Store).List(ctx, service.Destination)
				if err != nil {
					t.Fatal(err)
				}
				e, err := delivery.Compose(c.Request, messages[1])
				if err != nil || base.Send(ctx, e).Outcome != delivery.Delivered {
					t.Fatal("receipt fixture", err)
				}
				if _, err = s.db.Exec(`UPDATE delivery_outbox SET claim_expires_at=0`); err != nil {
					t.Fatal(err)
				}
			}
			held := heldDelivery{Adapter: base, query: query, enter: make(chan struct{}), leave: make(chan struct{})}
			var once sync.Once
			release := func() { once.Do(func() { close(held.leave) }) }
			defer release()
			service.Adapter = held
			service.Policy.LeaseSeconds = 1
			done := make(chan error, 1)
			go func() { done <- service.Drain(ctx) }()
			select {
			case <-held.enter:
			case <-ctx.Done():
				t.Fatal("transport did not start")
			}
			// Hold longer than the configured lease. Another consumer must
			// remain excluded while the original send/query is still pending.
			time.Sleep(1300 * time.Millisecond)
			if _, err := s.Deliveries().Claim(ctx, service.Channel, service.Destination, nil, time.Now(), time.Second); !errors.Is(err, delivery.ErrEmpty) {
				t.Fatal("live slow transport taken over", err)
			}
			release()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("transport/heartbeat did not join")
			}
			rows := deliveryRows(t, s)
			items, err := base.Messages(ctx, service.Destination)
			if err != nil || rows[0].Status != "sent" || rows[0].AttemptCount != 1 || len(items) != 1 {
				t.Fatal("slow delivery lost or repeated", rows, items, err)
			}
		})
	}
}

func TestDeliveryOwnershipLossCancelsAndDoesNotAbortConsumer(t *testing.T) {
	s, service, base := terminalFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	held := heldDelivery{Adapter: base, enter: make(chan struct{}), leave: make(chan struct{})}
	service.Adapter = held
	service.Policy.LeaseSeconds = 1
	done := make(chan error, 1)
	go func() { done <- service.Drain(ctx) }()
	select {
	case <-held.enter:
	case <-ctx.Done():
		t.Fatal("transport did not start")
	}
	if _, err := s.db.Exec(`UPDATE delivery_outbox SET claim_expires_at=0`); err != nil {
		t.Fatal(err)
	}
	// A competing consumer queries the uncertain old attempt, never resends.
	competitor := service
	transport := &uncertainTransport{observation: delivery.Observation{Outcome: delivery.Retry}}
	competitor.Adapter = transport
	if err := competitor.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("ownership loss stopped consumer", err)
		}
	case <-ctx.Done():
		t.Fatal("old transport/heartbeat did not cancel and join")
	}
	items, err := base.Messages(ctx, service.Destination)
	if err != nil || len(items) != 0 || transport.sends != 0 || deliveryRows(t, s)[0].Status != "unknown" {
		t.Fatal("lost owner wrote a receipt or recovery resent", items, err)
	}
}

func TestDeliveryLateSettlementLeavesReceiptForRecovery(t *testing.T) {
	s, service, base := terminalFixture(t)
	service.Boundary = func(name string) {
		if name == "before_delivery_ack" {
			if _, err := s.db.Exec(`UPDATE delivery_outbox SET claim_expires_at=0`); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := service.Drain(context.Background()); err != nil {
		t.Fatal("late settlement stopped consumer", err)
	}
	service.Boundary = nil
	if err := service.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	items, err := base.Messages(context.Background(), service.Destination)
	if err != nil || len(items) != 1 || deliveryRows(t, s)[0].Status != "sent" || deliveryRows(t, s)[0].AttemptCount != 1 {
		t.Fatal("receipt recovery lost or repeated delivery", items, err)
	}
}
