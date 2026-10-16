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
