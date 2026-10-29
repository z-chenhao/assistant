package bridge_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/z-chenhao/collie/internal/llm"
)

func TestDisposeAcknowledgementSettlesGenerateAndContinue(t *testing.T) {
	for _, operation := range []string{"generate", "continue"} {
		for _, mode := range []string{"ack-only", "pre-ack", "late", "completed"} {
			t.Run(operation+"/"+mode, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				client, _ := lifecyclePeer(t, ctx)
				r := request()
				r.Messages[0].Text = mode
				entered, release := make(chan struct{}), make(chan struct{})
				var once sync.Once
				unblock := func() { once.Do(func() { close(release) }) }
				defer unblock()
				type outcome struct {
					result llm.GenerationResult
					err    error
				}
				done := make(chan outcome, 1)
				go func() {
					sink := func(e llm.GenerationEvent) {
						if (mode == "completed" && e.Kind == "completed") || (mode != "completed" && e.Kind == "text_delta") {
							close(entered)
							if mode == "completed" || mode == "late" {
								<-release
							}
						}
					}
					var result llm.GenerationResult
					var err error
					if operation == "generate" {
						result, err = client.Generate(ctx, r, sink)
					} else {
						result, err = client.Continue(ctx, r, sink)
					}
					done <- outcome{result, err}
				}()
				if mode == "pre-ack" {
					for lifecycleState(t, ctx, client).ContextWindow == 0 {
						if ctx.Err() != nil {
							t.Fatal(ctx.Err())
						}
					}
				} else {
					select {
					case <-entered:
					case <-ctx.Done():
						t.Fatal("no stream barrier")
					}
				}
				if err := client.Dispose(ctx, r.SessionID); err != nil {
					t.Fatal(err)
				}
				if state := lifecycleState(t, ctx, client); state.ContextWindow != 0 || state.MaxOutputTokens != 0 {
					t.Fatal("disposal leaked work or required Cancel", state)
				}
				unblock()
				select {
				case got := <-done:
					if !errors.Is(got.err, llm.GenerationCancelled) || got.result.Text != "" || len(got.result.ToolCalls) != 0 || got.result.Usage != nil || ctx.Err() != nil {
						t.Fatal("disposed call published a result", got)
					}
				case <-time.After(time.Second):
					t.Fatal("call did not settle after disposal ack")
				}
				// The Run's terminal session cannot be revived by a late caller.
				r.GenerationID = "late-admission"
				if _, err := client.Generate(ctx, r, nil); !errors.Is(err, llm.GenerationCancelled) {
					t.Fatal("terminal session admitted work", err)
				}
				if state := lifecycleState(t, ctx, client); state.ContextWindow != 0 {
					t.Fatal("late admission reached peer", state)
				}
			})
		}
	}
}

func TestDisposeAfterPublishedCompletionPreservesResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, _ := lifecyclePeer(t, ctx)
	r := request()
	r.Messages[0].Text = "completed"
	result, err := client.Generate(ctx, r, nil)
	if err != nil || result.Text != "x" {
		t.Fatal(result, err)
	}
	if err := client.Dispose(ctx, r.SessionID); err != nil {
		t.Fatal(err)
	}
	if result.Text != "x" {
		t.Fatal("disposal changed published result")
	}
}

func TestDisposeFailureRetiresUnobservableWork(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, _ := lifecyclePeer(t, ctx)
	before := lifecycleState(t, ctx, client)
	r := request()
	r.Messages[0].Text = "dispose-no-ack"
	entered, done := make(chan struct{}), make(chan error, 1)
	go func() { _, err := client.Generate(ctx, r, func(llm.GenerationEvent) { close(entered) }); done <- err }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("no stream")
	}
	short, stop := context.WithTimeout(ctx, 50*time.Millisecond)
	defer stop()
	if err := client.Dispose(short, r.SessionID); !errors.Is(err, llm.ProviderUnavailable) {
		t.Fatal("unacknowledged disposal succeeded", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, llm.ProviderUnavailable) {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("failed disposal left blocked caller")
	}
	after := lifecycleState(t, ctx, client)
	if before.ModelID == after.ModelID || after.ContextWindow != 0 {
		t.Fatal("failed cleanup did not retire peer", after)
	}
}
