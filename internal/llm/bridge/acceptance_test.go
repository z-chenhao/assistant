package bridge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
)

func TestCancellationDeadlineDuringUsageCommit(t *testing.T) {
	for _, action := range []string{"cancel", "dispose"} {
		t.Run(action, func(t *testing.T) {
			ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
			defer stop()
			c := openCancelDispatchPeer(t, ctx)
			stream := newEventStream("held", 10)
			stream.sessionID = "run"
			close(stream.dispatchDone)
			c.p.mu.Lock()
			c.p.events["held"] = stream
			c.p.mu.Unlock()
			// The peer acknowledges Cancel immediately only after an admitted
			// start. This keeps the deadline probe at Host commit admission.
			if err := c.call(ctx, "generation.start", map[string]string{"generation_id": "held"}, nil); err != nil {
				t.Fatal(err)
			}
			entered, release, committed := make(chan struct{}), make(chan struct{}), make(chan error, 1)
			go func() {
				committed <- stream.finalize(func(bool) error {
					close(entered)
					<-release // deterministic storage delay, after commit admission
					return nil
				})
			}()
			<-entered
			wait, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
			defer cancel()
			ack := make(chan error, 1)
			go func() {
				if action == "cancel" {
					ack <- c.Cancel(wait, "held")
				} else {
					ack <- c.Dispose(wait, "run")
				}
			}()
			var err error
			acknowledged := false
			select {
			case err = <-ack:
				acknowledged = true
				if !errors.Is(err, llm.ProviderUnavailable) || wait.Err() == nil {
					t.Error("commit wait did not honor caller deadline", err, wait.Err())
				}
				// A pending commit must not hold the shared RPC reader. Failed
				// disposal may replace the peer, but catalog must still progress.
				if _, err := c.List(ctx); err != nil {
					t.Error("pending commit blocked independent RPC", err)
				}
			case <-time.After(250 * time.Millisecond):
				t.Error("cancellation waited beyond deadline for usage commit")
			}
			close(release)
			if err := <-committed; err != nil {
				t.Fatal(err)
			}
			if !acknowledged {
				<-ack // drain the timed-out path before ending this test
			}
			if stream.wasCancelled() {
				t.Fatal("failed cancellation changed an already admitted commit")
			}
		})
	}
}
