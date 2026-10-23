package bridge_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/llm/bridge"
	"github.com/z-chenhao/erii/internal/llm/fake"
)

func TestBridgeProcess(t *testing.T) {
	last := os.Args[len(os.Args)-1]
	if !strings.HasPrefix(last, "fixture:") {
		return
	}
	if err := fake.Serve(os.Stdin, os.Stdout, strings.TrimPrefix(last, "fixture:")); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func open(t *testing.T, fixture string) *bridge.Client {
	t.Helper()
	c, err := bridge.Open(context.Background(), os.Args[0], "-test.run=^TestBridgeProcess$", "--", "fixture:"+fixture)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
	return c
}

func request() llm.GenerationRequest {
	return llm.GenerationRequest{GenerationID: "generation", RunID: "run", SessionID: "run", Snapshot: llm.Snapshot{SnapshotID: "snapshot", MaxOutputTokens: 1024}, Messages: []llm.Message{{Role: "user", Text: "OK"}}}
}

func TestHandshakeAndSanitizedFailures(t *testing.T) {
	_, err := bridge.Open(context.Background(), os.Args[0], "-test.run=^TestBridgeProcess$", "--", "fixture:incompatible")
	if !errors.Is(err, llm.ModelIncompatible) {
		t.Fatalf("incompatible handshake: %v", err)
	}
	for _, tc := range []struct {
		fixture string
		want    error
	}{{"raw-error", llm.InvalidProviderResponse}, {"malformed", llm.InvalidProviderResponse}, {"auth-required", llm.AuthRequired}, {"bad-sequence", llm.InvalidProviderResponse}, {"drift", llm.InvalidProviderResponse}, {"crash", llm.ProviderUnavailable}} {
		t.Run(tc.fixture, func(t *testing.T) {
			c := open(t, tc.fixture)
			_, err := c.Generate(context.Background(), request(), nil)
			if !errors.Is(err, tc.want) || strings.Contains(err.Error(), "SYNTHETIC") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestStableProviderErrorsAndOutputBounds(t *testing.T) {
	for _, code := range []llm.ProviderError{llm.AuthRequired, llm.ModelNotFound, llm.ModelIncompatible, llm.RateLimited, llm.ContextLengthExceeded, llm.ProviderUnavailable, llm.GenerationCancelled, llm.InvalidProviderResponse} {
		t.Run(string(code), func(t *testing.T) {
			c := open(t, "error:"+string(code))
			result, err := c.Generate(context.Background(), request(), nil)
			if !errors.Is(err, code) || result.Text != "" {
				t.Fatalf("provider error drift: %+v %v", result, err)
			}
		})
	}
	c := open(t, "event-error")
	if result, err := c.Generate(context.Background(), request(), nil); !errors.Is(err, llm.RateLimited) || result.Text != "" {
		t.Fatal(result, err)
	}
	c = open(t, "success")
	r := request()
	r.Messages[0].Text = "Produce a long output for clipping."
	bytes := 0
	result, err := c.Generate(context.Background(), r, func(e llm.GenerationEvent) { bytes += len(e.Text) })
	if err != nil || !result.Clipped || len(result.Text) != 4096 || bytes != 4096 {
		t.Fatalf("output exceeded bound: %d %d %+v %v", len(result.Text), bytes, result.Clipped, err)
	}
	ctx, stop := context.WithCancel(context.Background())
	stop()
	if _, err := c.Generate(ctx, r, nil); !errors.Is(err, llm.GenerationCancelled) {
		t.Fatalf("cancel before admission: %v", err)
	}
}

func TestDirectCancelEndsWaitingGeneration(t *testing.T) {
	c := open(t, "success")
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	r := request()
	r.Messages[0].Text = "Stream until cancelled."
	started := make(chan struct{})
	done := make(chan error, 1)
	var once sync.Once
	go func() {
		_, err := c.Generate(ctx, r, func(e llm.GenerationEvent) { once.Do(func() { close(started) }) })
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("generation did not stream")
	}
	if err := c.Cancel(ctx, r.GenerationID); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, llm.GenerationCancelled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Cancel acknowledged but generation still waiting")
	}
}

func TestProbeAndAuthCatalogContract(t *testing.T) {
	ctx := context.Background()
	c := open(t, "success")
	a, err := c.Start(ctx)
	if err != nil || len(a.Events) != 5 || a.Events[0].Kind != "open_url" || a.Events[1].Kind != "device_code" || a.Events[2].Kind != "progress" || a.Events[3].Kind != "choose_workspace" || a.Events[4].Kind != "input_required" {
		t.Fatalf("UI-neutral events: %+v %v", a, err)
	}
	for _, fn := range []func(context.Context) (llm.AuthState, error){func(ctx context.Context) (llm.AuthState, error) { return c.ContinueAuth(ctx, "test-workspace") }, c.Refresh, c.Status} {
		a, err := fn(ctx)
		if err != nil || a.Status != "connected" {
			t.Fatal(a, err)
		}
	}
	if err := c.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	a, err = c.Status(ctx)
	if err != nil || a.Status != "logged_out" {
		t.Fatal(a, err)
	}
	for _, fn := range []func(context.Context) ([]llm.CatalogModel, error){c.List, c.RefreshCatalog} {
		models, err := fn(ctx)
		if err != nil || len(models) != 1 || models[0].ModelID != "fixture-model" {
			t.Fatal(models, err)
		}
	}
	p := llm.Profile{ProfileID: "profile", Version: 1, ProfileInput: llm.ProfileInput{Endpoint: "http://localhost/v1", MaxOutputTokens: 1024}}
	var stages []llm.ProbeStage
	result, err := c.RunWithProgress(ctx, p, func(stage llm.ProbeStage) { stages = append(stages, stage) })
	if err != nil || result.Validate() != nil {
		t.Fatalf("behavioral Probe: %+v %v", result, err)
	}
	if len(stages) != 5 || stages[0] != llm.ProbeConnection || stages[4] != llm.ProbeClipping {
		t.Fatalf("Bridge dropped Probe progress: %v", stages)
	}
	noTools := open(t, "no-tools")
	if _, err := (llm.Prober{Provider: noTools}).Run(ctx, p); !errors.Is(err, llm.ModelIncompatible) {
		t.Fatalf("tool-less model ready: %v", err)
	}
}

func TestDedupCompletionCancelAndRestart(t *testing.T) {
	ctx := context.Background()
	c := open(t, "success")
	count := map[string]int{}
	r, err := c.Generate(ctx, request(), func(e llm.GenerationEvent) { count[e.Kind]++ })
	if err != nil || r.Text != "OK" || count["text_delta"] != 1 || count["completed"] != 1 {
		t.Fatalf("duplicate adoption: %+v %+v %v", r, count, err)
	}
	// Concurrent cancellation uses the same RPC process while generation waits.
	cancelRequest := request()
	cancelRequest.GenerationID = "cancel"
	cancelRequest.Messages[0].Text = "Stream until cancelled."
	started := make(chan struct{})
	done := make(chan error, 1)
	var once sync.Once
	genCtx, stop := context.WithCancel(ctx)
	go func() {
		_, err := c.Generate(genCtx, cancelRequest, func(e llm.GenerationEvent) { once.Do(func() { close(started) }) })
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("generation did not start")
	}
	stop()
	select {
	case err := <-done:
		if !errors.Is(err, llm.GenerationCancelled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation hung")
	}
	crash := open(t, "crash")
	if _, err := crash.Generate(ctx, request(), nil); !errors.Is(err, llm.ProviderUnavailable) {
		t.Fatal(err)
	}
	// No partial result is adopted, no generation retry. The next metadata call
	// restarts and re-handshakes the same executable/version.
	models, err := crash.List(ctx)
	if err != nil || len(models) != 1 {
		t.Fatalf("restart failed: %v", err)
	}
	partial := open(t, "partial-crash")
	if result, err := partial.Generate(ctx, request(), nil); !errors.Is(err, llm.ProviderUnavailable) || result.Text != "" {
		t.Fatalf("partial crash adopted output: %+v %v", result, err)
	}
	completed := open(t, "completed-crash")
	if result, err := completed.Generate(ctx, request(), nil); err != nil || result.Text != "OK" {
		t.Fatalf("lost received completion at EOF: %+v %v", result, err)
	}
}
