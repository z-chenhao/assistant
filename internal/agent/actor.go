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
	type completed struct {
		id  string
		err error
	}
	done := make(chan completed, 5)
	running := map[string]bool{}
	var firstErr error
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		found := false
		if ctx.Err() == nil && firstErr == nil && len(running) < 5 {
			id, err := a.Store.Next(ctx)
			if err == nil && !running[id] {
				claim, err := a.Store.Claim(ctx, id)
				if err == nil {
					running[id] = true
					found = true
					go func() {
						run, loadErr := a.Store.Load(ctx, claim)
						err := loadErr
						if err == nil {
							err = a.execute(ctx, id, claim)
						}
						// A Root's business failure is durably reported to Main. Infrastructure
						// failures still stop this drain after all owned work has been joined.
						var failed *runFailure
						if errors.As(err, &failed) {
							if run.Kind == "worker" {
								err = nil
							} else {
								err = failed.cause
							}
						}
						done <- completed{id, err}
					}()
				} else if !errors.Is(err, ErrBusy) {
					firstErr = err
				}
			} else if err != nil && !errors.Is(err, ErrBusy) && !errors.Is(err, ErrNotFound) {
				firstErr = err
			}
			if errors.Is(err, ErrNotFound) && len(running) == 0 {
				return firstErr
			}
		}
		if found {
			continue
		}
		if len(running) == 0 && (firstErr != nil || ctx.Err() != nil) {
			if firstErr != nil {
				return firstErr
			}
			return ctx.Err()
		}
		select {
		case result := <-done:
			delete(running, result.id)
			if result.err != nil && firstErr == nil {
				firstErr = result.err
			}
		case <-ticker.C:
		}
	}
}
func (a Actor) execute(ctx context.Context, id string, claim RunClaim) error {
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
	err := a.Store.Initialize(work, claim)
	if err == nil {
		run, loadErr := a.Store.Load(work, claim)
		if loadErr != nil {
			err = loadErr
		} else {
			var registry *tool.Registry
			registry, err = a.Registry.Select(run.Tools)
			if err == nil {
				executor := tool.Executor{Registry: registry, Store: a.Store.ToolStore(claim), Concurrency: 4, Sink: func(e tool.Event) {
					emit(Event{Kind: e.Kind, RunID: id, OccurredAt: time.Now().UTC(), ToolCallID: e.Call.ID})
				}}
				err = RunAgentLoop(work, claim, LoopOptions{Store: a.Store, Provider: a.Provider, Tools: executor, Sink: emit})
			}
		}
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
	return nil
}
