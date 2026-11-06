package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/task"
)

func TestSerialMainRetainsBothRequestsAfterReopeningBeforeFirstReply(t *testing.T) {
	s, runs, history, actor, provider := loopFixture(t)
	ctx := context.Background()
	first := mustSubmit(t, actor, "first", "first request")
	claim, err := runs.Claim(ctx, first.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err = runs.Initialize(ctx, claim); err != nil {
		t.Fatal(err)
	}
	run, err := runs.Load(ctx, claim)
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := runs.Steering(ctx, claim, 0)
	if err != nil || len(inputs) != 1 {
		t.Fatal("first input unavailable", inputs, err)
	}
	run.Checkpoint.Messages = append(run.Checkpoint.Messages, agent.AgentMessage{Type: "user", MessageID: inputs[0].Message.MessageID, Content: inputs[0].Message.Content})
	run.Checkpoint.InputSequence = inputs[0].Sequence
	if err = runs.Checkpoint(ctx, claim, run.Version, run.Checkpoint); err != nil {
		t.Fatal(err)
	}
	second := mustSubmit(t, actor, "second", "second request")
	if second.RunID != first.RunID {
		t.Fatal("queued input started a competing Main")
	}
	if err = runs.Release(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	recovered, err := Open(ctx, filepath.Dir(s.path))
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	actor.Store = recovered.Runs(history, actor.Registry)
	provider.store = recovered
	provider.respond = func(request llm.GenerationRequest) (llm.GenerationResult, error) {
		return llm.GenerationResult{Text: "reply to " + request.Messages[len(request.Messages)-1].Text}, nil
	}
	mustDrain(t, actor)
	if len(provider.requests) != 2 || scalar(t, recovered, `SELECT count(*) FROM main_reply`) != 2 {
		t.Fatal("recovery did not answer both inputs exactly once")
	}
	for _, message := range provider.requests[0].Messages {
		if message.Role == "user" && message.Text == "second request" {
			t.Fatal("second input replaced the unreplied first input after recovery")
		}
	}
	found := false
	for _, message := range provider.requests[1].Messages {
		found = found || message.Role == "assistant" && message.Text == "reply to first request"
	}
	if !found {
		t.Fatal("second input ran before the first reply was retained")
	}
}

func TestMainQueuesSecondRequestUntilFirstTaskHandoffReply(t *testing.T) {
	s, _, a, p := taskFixture(t)
	firstStarted, firstRelease, workerRelease := make(chan struct{}), make(chan struct{}), make(chan struct{})
	replies := make(chan struct{}, 2)
	a.Sink = func(event agent.Event) {
		if event.Kind == "reply_committed" {
			select {
			case replies <- struct{}{}:
			default:
			}
		}
	}
	var first, releaseFirst, releaseWorker sync.Once
	p.respond = func(request llm.GenerationRequest) (llm.GenerationResult, error) {
		if isRoot(request) {
			<-workerRelease
			return llm.GenerationResult{Text: "game delivered"}, nil
		}
		if isFollowup(request) {
			return llm.GenerationResult{Text: "game delivered"}, nil
		}
		last := request.Messages[len(request.Messages)-1]
		if last.Role == "tool" {
			return llm.GenerationResult{Text: "game started in background"}, nil
		}
		if last.Role == "user" && last.Text == "make a game" {
			first.Do(func() { close(firstStarted); <-firstRelease })
			return calls(task.ManageName, `{"operation":"create","task":"make a game"}`), nil
		}
		if last.Role == "user" && last.Text == "recommend a snowy trip" {
			return llm.GenerationResult{Text: "trip recommendation"}, nil
		}
		return llm.GenerationResult{}, llm.InvalidProviderResponse
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	mustSubmit(t, a, "game", "make a game")
	go func() { done <- a.Drain(ctx) }()
	t.Cleanup(func() {
		releaseFirst.Do(func() { close(firstRelease) })
		releaseWorker.Do(func() { close(workerRelease) })
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("executor did not join")
		}
	})
	select {
	case <-firstStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("first request did not start")
	}
	mustSubmit(t, a, "trip", "recommend a snowy trip")
	releaseFirst.Do(func() { close(firstRelease) })
	// Ordering is the contract: both replies must commit while the Worker is
	// still held. Avoid contending with instrumented writes through a 10ms SQL
	// polling loop; the guard is for hung tests, not a production latency SLA.
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	for range 2 {
		select {
		case <-replies:
		case err := <-done:
			done <- err
			t.Fatal("executor exited before both replies", err)
		case <-deadline.C:
			t.Fatal("second request lost the first handoff or waited for the Worker")
		}
	}
	if scalar(t, s, `SELECT count(*) FROM main_reply`) != 2 {
		t.Fatal("reply notification preceded durable commit")
	}
	if scalar(t, s, `SELECT count(*) FROM task`) != 1 || scalar(t, s, `SELECT count(*) FROM run WHERE kind='worker' AND status IN('queued','running')`) != 1 {
		t.Fatal("game was not delegated exactly once")
	}
	p.mu.Lock()
	seenTrip := false
	for _, request := range p.requests {
		if isRoot(request) || isFollowup(request) {
			continue
		}
		for _, message := range request.Messages {
			if message.Role == "user" && message.Text == "recommend a snowy trip" {
				seenTrip = true
			}
		}
		if seenTrip {
			found := false
			for _, message := range request.Messages {
				found = found || message.Role == "assistant" && strings.Contains(message.Text, "game started in background")
			}
			if !found {
				p.mu.Unlock()
				t.Fatal("second request entered the model before the first handoff reply")
			}
		}
	}
	p.mu.Unlock()
	releaseWorker.Do(func() { close(workerRelease) })
	select {
	case err := <-done:
		done <- err
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Worker did not complete")
	}
	if scalar(t, s, `SELECT count(*) FROM task WHERE status='completed'`) != 1 {
		t.Fatal("background Task result was not retained")
	}
}

// Exercise recovery of pre-change Runs without changing their frozen policy.
func legacyMainInputs(t *testing.T, r *Runs, id string) {
	t.Helper()
	ctx := context.Background()
	claim, err := r.Claim(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	run, err := r.Load(ctx, claim)
	if err != nil {
		t.Fatal(err)
	}
	cp := run.Checkpoint
	cp.SerialInputs = false
	if err = r.Checkpoint(ctx, claim, run.Version, cp); err != nil {
		t.Fatal(err)
	}
	if err = r.Release(ctx, claim); err != nil {
		t.Fatal(err)
	}
}
