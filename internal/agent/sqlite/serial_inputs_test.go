package sqlite

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/task"
)

func TestMainQueuesSecondRequestUntilFirstTaskHandoffReply(t *testing.T) {
	s, _, a, p := taskFixture(t)
	firstStarted, firstRelease, workerRelease := make(chan struct{}), make(chan struct{}), make(chan struct{})
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
	deadline := time.Now().Add(5 * time.Second)
	for scalar(t, s, `SELECT count(*) FROM main_reply`) < 2 {
		if time.Now().After(deadline) {
			t.Fatal("second request lost the first handoff or waited for the Worker")
		}
		time.Sleep(10 * time.Millisecond)
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
