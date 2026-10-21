package web

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestDeliveryRunsWhileWorkerBusyAndShutdownJoins(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	started, delivered, joined := make(chan struct{}), make(chan struct{}, 1), make(chan struct{})
	s := &Server{}
	result := make(chan error, 1)
	go func() {
		result <- Run(ctx, s, listener, func(ctx context.Context) error { close(started); <-ctx.Done(); close(joined); return ctx.Err() }, func(context.Context) error {
			select {
			case delivered <- struct{}{}:
			default:
			}
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Actor not started")
	}
	select {
	case <-delivered:
	case <-time.After(time.Second):
		t.Fatal("active Worker blocked delivery")
	}
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not join owned loops")
	}
	select {
	case <-joined:
	default:
		t.Fatal("Actor survived shutdown")
	}
}
