package agent

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/tool"
)

type Receipt struct {
	RequestKey string `json:"request_key"`
	RunID      string `json:"run_id"`
	MessageID  string `json:"message_id"`
}
type ActorStore interface {
	RunStore
	Submit(context.Context, string, []conversation.ContentPart) (Receipt, error)
	Next(context.Context) (string, error)
	Claim(context.Context, string) (RunClaim, error)
	Renew(context.Context, RunClaim) error
	Release(context.Context, RunClaim) error
	Initialize(context.Context, RunClaim) error
	ToolStore(RunClaim) tool.Store
	Trace(context.Context, RunClaim, Event) (Event, error)
}

// Actor coordinates ownership and lifecycle. All reasoning remains in loop.go.
// Database uniqueness, rather than this object's lifetime, enforces one Main.
type Actor struct {
	Store    ActorStore
	Provider llm.LLMProvider
	Registry *tool.Registry
	Ready    func(context.Context) bool
	Sink     EventSink
}

func (a Actor) Submit(ctx context.Context, key, text string) (Receipt, error) {
	if a.Ready == nil || !a.Ready(ctx) {
		return Receipt{}, ErrSetup
	}
	if key == "" || len(key) > 256 || text == "" || len(text) > 16<<10 {
		return Receipt{}, conversation.ErrInvalid
	}
	return a.Store.Submit(ctx, key, conversation.Text(text))
}

// Drain handles all currently accepted work in the foreground. A later call or
// process restart scans the same authoritative rows; queue delivery is a wakeup.
func (a Actor) Drain(ctx context.Context) error {
	if a.Provider == nil {
		return ErrSetup
	}
	for ctx.Err() == nil {
		id, err := a.Store.Next(ctx)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		claim, err := a.Store.Claim(ctx, id)
		if errors.Is(err, ErrBusy) {
			timer := time.NewTimer(100 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if err != nil {
			return err
		}
		work, stop := context.WithCancel(ctx)
		done := make(chan struct{})
		var heartbeat sync.WaitGroup
		heartbeat.Add(1)
		go func() {
			defer heartbeat.Done()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					if a.Store.Renew(context.WithoutCancel(work), claim) != nil {
						stop()
						return
					}
				}
			}
		}()
		emit := func(e Event) {
			event, err := a.Store.Trace(context.WithoutCancel(work), claim, e)
			if err == nil && a.Sink != nil {
				a.Sink(event)
			}
		}
		err = a.Store.Initialize(work, claim)
		if err == nil {
			executor := tool.Executor{Registry: a.Registry, Store: a.Store.ToolStore(claim), Concurrency: 4, Sink: func(e tool.Event) {
				emit(Event{Kind: e.Kind, RunID: id, OccurredAt: time.Now().UTC(), ToolCallID: e.Call.ID})
			}}
			err = RunAgentLoop(work, claim, LoopOptions{Store: a.Store, Provider: a.Provider, Tools: executor, Sink: emit})
		}
		close(done)
		heartbeat.Wait()
		stop()
		// Terminal Runs have already relinquished ownership. Boundary stops
		// release the lease and keep the nonterminal Run recoverable.
		releaseErr := a.Store.Release(context.WithoutCancel(ctx), claim)
		if err != nil {
			return err
		}
		if releaseErr != nil && !errors.Is(releaseErr, ErrClaimLost) {
			return releaseErr
		}
	}
	return ctx.Err()
}
