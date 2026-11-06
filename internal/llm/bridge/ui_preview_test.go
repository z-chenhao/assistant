package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/assistant/internal/llm"
)

func TestNativeBridgeUIPreviewBeforeToolCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	partial := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunk := func(delta any, finish any) {
			b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
			fmt.Fprintf(w, "data: %s\n\n", b)
			w.(http.Flusher).Flush()
		}
		prefix := `{"title":"Preview","components":[`
		chunk(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "ui-call", "type": "function", "function": map[string]any{"name": "builtin__intelligent_ui_show", "arguments": prefix}}}}, nil)
		select {
		case <-partial:
		case <-r.Context().Done():
			return
		}
		chunk(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "function": map[string]any{"arguments": `]}`}}}}, nil)
		// An unrelated tool's parameters must not acquire a preview event.
		chunk(map[string]any{"tool_calls": []any{map[string]any{"index": 1, "id": "private-call", "type": "function", "function": map[string]any{"name": "private_tool", "arguments": `{"secret":"hidden"}`}}}}, nil)
		chunk(map[string]any{}, "tool_calls")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	command, argument := nativeBridgeProcess(t)
	endpoint := server.URL + "/v1"
	client, err := OpenLocal(ctx, command, argument, func(context.Context, llm.Snapshot) (llm.Connection, error) {
		return llm.Connection{Endpoint: endpoint}, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	hash, _ := llm.EndpointHash(endpoint)
	request := llm.GenerationRequest{GenerationID: "ui", RunID: "run", SessionID: "run", Snapshot: llm.Snapshot{SnapshotID: "s", ProviderKind: "openai_compatible", WireAPI: "openai_completions", EndpointHash: hash, ModelID: "fixture-model", ContextWindow: 8192, MaxOutputTokens: 1024, Compatibility: llm.Compatibility{ToolResultFormat: "openai"}}, Messages: []llm.Message{{Role: "user", Text: "show UI"}}}
	var delta strings.Builder
	result, err := client.Generate(ctx, request, func(e llm.GenerationEvent) {
		if e.Kind == "ui_delta" {
			if e.ToolCall == nil || e.ToolCall.Name != "builtin__intelligent_ui_show" {
				t.Error(e)
				return
			}
			delta.WriteString(e.ToolCall.Arguments)
			select {
			case partial <- struct{}{}:
			default:
			}
		}
	})
	if err != nil || len(result.ToolCalls) != 2 || delta.String() != `{"title":"Preview","components":[]}` {
		t.Fatal(result, err, delta.String())
	}
}
func TestUIPreviewMailboxRejectsOtherToolsAndChangedDuplicates(t *testing.T) {
	s := newEventStream("g", 1024)
	event := llm.GenerationEvent{Sequence: 1, Kind: "ui_delta", ToolCall: &llm.ToolCall{ID: "call", Name: "builtin__intelligent_ui_show", Arguments: `{"title":`}}
	s.accept(event)
	s.accept(event)
	event.ToolCall = &llm.ToolCall{ID: "call", Name: "builtin__intelligent_ui_show", Arguments: `{"other":`}
	s.accept(event)
	if len(s.events) != 2 {
		t.Fatal("preview duplicate validation changed", len(s.events))
	}
	s = newEventStream("g", 1024)
	event.ToolCall.Name = "private_tool"
	s.accept(event)
	if d := <-s.events; d.err == nil {
		t.Fatal("unrelated arguments became UI preview")
	}
}

func TestUIPreviewRequiresRPCOptInForLegacyV1Clients(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		delta := map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "ui", "type": "function", "function": map[string]any{"name": "builtin__intelligent_ui_show", "arguments": `{"title":"A"}`}}}}
		for _, choice := range []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}, map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"}} {
			b, _ := json.Marshal(map[string]any{"choices": []any{choice}})
			fmt.Fprintf(w, "data: %s\n\n", b)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer peer.Close()
	endpoint := peer.URL + "/v1"
	hash, _ := llm.EndpointHash(endpoint)
	request := llm.GenerationRequest{GenerationID: "g", RunID: "r", SessionID: "r", Snapshot: llm.Snapshot{SnapshotID: "s", ProviderKind: "openai_compatible", WireAPI: "openai_completions", EndpointHash: hash, ModelID: "fixture", ContextWindow: 8192, MaxOutputTokens: 1024, Compatibility: llm.Compatibility{ToolResultFormat: "openai"}}, Messages: []llm.Message{{Role: "user", Text: "show"}}}
	for _, enabled := range []bool{false, true} {
		var out bytes.Buffer
		server := &Server{out: &out}
		server.generate(context.Background(), generationRequest{GenerationRequest: request, Connection: llm.Connection{Endpoint: endpoint}, UIPreview: enabled}, &job{})
		if strings.Contains(out.String(), `"kind":"ui_delta"`) != enabled || !strings.Contains(out.String(), `"kind":"completed"`) {
			t.Fatal("v1 event compatibility changed", enabled, out.String())
		}
	}
}
