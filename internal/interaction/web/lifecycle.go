package web

import (
	"context"
	"net"
	"time"
)

// Run joins ingress, the existing Actor and independent transport recovery.
// A busy Worker cannot hold back another Run's approval or final notification.
func Run(ctx context.Context, s *Server, l net.Listener, drain, deliver func(context.Context) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	wake := make(chan struct{}, 1)
	s.Wake = func() {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	actorDone, deliveryDone := make(chan error, 1), make(chan error, 1)
	run := func(action func(context.Context) error, done chan<- error, ingress bool) {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			if err := action(ctx); err != nil {
				if ctx.Err() != nil {
					done <- nil
				} else {
					done <- err
					cancel()
				}
				return
			}
			if ingress {
				select {
				case <-ctx.Done():
					done <- nil
					return
				case <-ticker.C:
				case <-wake:
				}
			} else {
				select {
				case <-ctx.Done():
					done <- nil
					return
				case <-ticker.C:
				}
			}
		}
	}
	go run(drain, actorDone, true)
	go run(deliver, deliveryDone, false)
	err := s.Serve(ctx, l)
	cancel()
	actorErr, deliveryErr := <-actorDone, <-deliveryDone
	if err != nil {
		return err
	}
	if actorErr != nil {
		return actorErr
	}
	return deliveryErr
}
