package bridge

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
)

func TestActualPiBridgeClippingCrashAndRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var partial atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("fabricated credential")
		}
		if r.URL.Path == "/v1/models" {
			fmt.Fprint(w, `{"data":[{"id":"fixture-model"}]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ABCDEFGHIJKLMNOPQRSTUVWXYZ\"},\"finish_reason\":null}]}\n\n")
		w.(http.Flusher).Flush()
		if partial.Load() {
			<-r.Context().Done()
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":8,\"total_tokens\":15}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	script, err := filepath.Abs("../../../provider-bridge/bridge.mjs")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := server.URL + "/v1"
	client, err := OpenLocal(ctx, "node", script, func(context.Context, llm.Snapshot) (Connection, error) { return Connection{Endpoint: endpoint}, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	hash, _ := llm.EndpointHash(endpoint)
	r := llm.GenerationRequest{GenerationID: "one", RunID: "run", SessionID: "run", OutputLimitBytes: 8, Snapshot: llm.Snapshot{SnapshotID: "s", ProviderKind: "openai_compatible", WireAPI: "openai_completions", EndpointHash: hash, ModelID: "fixture-model", ContextWindow: 8192, MaxOutputTokens: 1024, Compatibility: llm.Compatibility{ToolResultFormat: "openai"}}, Messages: []llm.Message{{Role: "user", Text: "hello"}}}
	result, err := client.Generate(ctx, r, nil)
	if err != nil || !result.Clipped || result.Text != "ABCDEFGH" || result.Usage == nil || result.Usage.InputTokens != 7 {
		t.Fatal("actual Pi clipping/usage", result, err)
	}
	partial.Store(true)
	r.GenerationID = "two"
	result, err = client.Generate(ctx, r, func(e llm.GenerationEvent) {
		if e.Kind == "text_delta" {
			client.mu.Lock()
			process := client.p
			client.mu.Unlock()
			if err := process.cmd.Process.Kill(); err != nil {
				t.Error(err)
			}
		}
	})
	if err != llm.ProviderUnavailable || result.Text != "" {
		t.Fatal("partial crash published completion", result, err)
	}
	models, err := client.CatalogAt(ctx, Connection{Endpoint: endpoint})
	if err != nil || len(models) != 1 || models[0].ModelID != "fixture-model" {
		t.Fatal("same Bridge restart", models, err)
	}
	if err := client.Dispose(ctx, "run"); err != nil {
		t.Fatal(err)
	}
}

func TestActualPiDisposeSettlesMatchingGenerateAndContinue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	closed := make(chan string, 3)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		closed <- r.Header.Get("session_id")
	}))
	defer server.Close()
	script, _ := filepath.Abs("../../../provider-bridge/bridge.mjs")
	endpoint := server.URL + "/v1"
	client, err := OpenLocal(ctx, "node", script, func(context.Context, llm.Snapshot) (Connection, error) { return Connection{Endpoint: endpoint}, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	hash, _ := llm.EndpointHash(endpoint)
	base := llm.GenerationRequest{RunID: "disposed", SessionID: "disposed", Snapshot: llm.Snapshot{SnapshotID: "s", ProviderKind: "openai_compatible", WireAPI: "openai_completions", EndpointHash: hash, ModelID: "fixture-model", ContextWindow: 8192, MaxOutputTokens: 1024, Compatibility: llm.Compatibility{ToolResultFormat: "openai"}}, Messages: []llm.Message{{Role: "user", Text: "hello"}}}
	type outcome struct {
		id     string
		result llm.GenerationResult
		err    error
	}
	entered, done := make(chan string, 3), make(chan outcome, 3)
	for _, id := range []string{"generate", "continue", "other"} {
		r := base
		r.GenerationID = id
		if id == "other" {
			r.RunID = "other"
			r.SessionID = "other"
		}
		go func() {
			sink := func(e llm.GenerationEvent) {
				if e.Kind == "text_delta" {
					entered <- r.GenerationID
				}
			}
			var result llm.GenerationResult
			var err error
			if r.GenerationID == "continue" {
				result, err = client.Continue(ctx, r, sink)
			} else {
				result, err = client.Generate(ctx, r, sink)
			}
			done <- outcome{r.GenerationID, result, err}
		}()
	}
	for range 3 {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("no stream")
		}
	}
	if err := client.Dispose(ctx, "disposed"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		select {
		case got := <-done:
			if got.id == "other" || got.err != llm.GenerationCancelled || got.result.Text != "" {
				t.Fatal("wrong disposal outcome", got)
			}
		case <-time.After(time.Second):
			t.Fatal("disposal left caller blocked")
		}
		select {
		case session := <-closed:
			if session != "disposed" {
				t.Fatal("unrelated HTTP request aborted")
			}
		case <-ctx.Done():
			t.Fatal("HTTP was not closed")
		}
	}
	select {
	case got := <-done:
		t.Fatal("unrelated call settled", got)
	default:
	}
	if err := client.Cancel(ctx, "other"); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got.id != "other" || got.err != llm.GenerationCancelled {
			t.Fatal(got)
		}
	case <-ctx.Done():
		t.Fatal("other Cancel blocked")
	}
	base.GenerationID = "revive"
	if _, err := client.Generate(ctx, base, nil); err != llm.GenerationCancelled {
		t.Fatal("revived disposed session", err)
	}
	if requests.Load() != 3 {
		t.Fatal("disposal replayed/admitted a model request", requests.Load())
	}
}
