package provider

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func awsFrame(kind, payload string) []byte {
	name := ":event-type"
	header := append([]byte{byte(len(name))}, []byte(name)...)
	header = append(header, 7, byte(len(kind)>>8), byte(len(kind)))
	header = append(header, []byte(kind)...)
	b := make([]byte, 12)
	binary.BigEndian.PutUint32(b, uint32(16+len(header)+len(payload)))
	binary.BigEndian.PutUint32(b[4:], uint32(len(header)))
	binary.BigEndian.PutUint32(b[8:], crc32.ChecksumIEEE(b[:8]))
	b = append(b, header...)
	b = append(b, []byte(payload)...)
	b = binary.BigEndian.AppendUint32(b, crc32.ChecksumIEEE(b))
	return b
}
func TestProtocolStreams(t *testing.T) {
	for _, api := range APIs() {
		t.Run(string(api), func(t *testing.T) {
			var captured map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if json.NewDecoder(r.Body).Decode(&captured) != nil {
					t.Error("invalid body")
				}
				send := func(v string) { fmt.Fprintf(w, "data: %s\n\n", v) }
				switch api {
				case OpenAICompletions, MistralConversations:
					if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
						t.Error(r.URL.Path)
					}
					send(`{"choices":[{"delta":{"content":"hello"}}]}`)
					send(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call1","function":{"name":"echo","arguments":"{\"x\":"}}]}}]}`)
					send(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":4,"completion_tokens":3}}`)
				case OpenAIResponses, AzureResponses, CodexResponses:
					if !strings.HasSuffix(r.URL.Path, "/responses") {
						t.Error(r.URL.Path)
					}
					if api == AzureResponses && r.Header.Get("api-key") != "key" {
						t.Error("Azure auth")
					}
					send(`{"type":"response.created","response":{"id":"resp1"}}`)
					send(`{"type":"response.output_text.delta","delta":"hello"}`)
					send(`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","call_id":"call1","name":"echo"}}`)
					send(`{"type":"response.function_call_arguments.delta","output_index":1,"delta":"{\"x\":1}"}`)
					send(`{"type":"response.output_item.done","output_index":1,"item":{"type":"function_call","call_id":"call1","name":"echo","arguments":"{\"x\":1}"}}`)
					send(`{"type":"response.completed","response":{"id":"resp1","usage":{"input_tokens":4,"output_tokens":3}}}`)
				case AnthropicMessages:
					if r.Header.Get("x-api-key") != "key" || r.Header.Get("Authorization") != "" {
						t.Error("Anthropic auth")
					}
					send(`{"type":"message_start","message":{"usage":{"input_tokens":4}}}`)
					send(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`)
					send(`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call1","name":"echo","input":{}}}`)
					send(`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"x\":1}"}}`)
					send(`{"type":"content_block_stop","index":1}`)
					send(`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":3}}`)
					send(`{"type":"message_stop"}`)
				case GoogleGenerative, GoogleVertex:
					if !strings.HasSuffix(r.URL.Path, ":streamGenerateContent") || r.URL.Query().Get("alt") != "sse" {
						t.Error(r.URL)
					}
					send(`{"candidates":[{"content":{"parts":[{"text":"hello"},{"functionCall":{"id":"call1","name":"echo","args":{"x":1}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":3}}`)
				case BedrockConverse:
					if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=test/") {
						t.Error("SigV4 missing")
					}
					for _, v := range [][2]string{{"contentBlockDelta", `{"contentBlockIndex":0,"delta":{"text":"hello"}}`}, {"contentBlockStart", `{"contentBlockIndex":1,"start":{"toolUse":{"toolUseId":"call1","name":"echo"}}}`}, {"contentBlockDelta", `{"contentBlockIndex":1,"delta":{"toolUse":{"input":"{\"x\":1}"}}}`}, {"contentBlockStop", `{"contentBlockIndex":1}`}, {"messageStop", `{"stopReason":"tool_use"}`}, {"metadata", `{"usage":{"inputTokens":4,"outputTokens":3}}`}} {
						w.Write(awsFrame(v[0], v[1]))
					}
				case GatewayMessages:
					send(`{"type":"text_delta","delta":"hello"}`)
					send(`{"type":"toolcall_end","contentIndex":1,"toolCall":{"id":"call1","name":"echo","arguments":{"x":1}}}`)
					send(`{"type":"done","message":{"usage":{"input":4,"output":3}}}`)
				}
			}))
			defer server.Close()
			events := []string{}
			model := Model{ID: "test", API: api, Provider: "test", BaseURL: server.URL, MaxTokens: 100, Cost: CostRates{Input: 1, Output: 2}}
			input := Context{SystemPrompt: "system", Messages: []Message{{Role: "user", Content: []Content{Text("hi")}}}, Tools: []Tool{{Name: "echo", Parameters: json.RawMessage(`{"type":"object","properties":{"x":{"type":"integer"}}}`)}}}
			result, err := (&Client{}).Stream(context.Background(), model, input, Options{APIKey: "key", AWS: AWSCredentials{AccessKey: "test", SecretKey: "secret", Region: "us-east-1"}}, func(e Event) { events = append(events, e.Type) })
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Content) != 2 || result.Content[0].Text != "hello" || result.Content[1].Name != "echo" || result.Content[1].Arguments["x"] != float64(1) {
				t.Fatalf("content %#v", result.Content)
			}
			if result.StopReason != "toolUse" || result.Usage.Input != 4 || result.Usage.Output != 3 || result.Usage.TotalTokens != 7 || !result.Usage.Known || result.Usage.Cost.Input != 4e-6 {
				t.Fatalf("result %#v", result)
			}
			if !strings.Contains(strings.Join(events, ","), "toolcall_end") || events[len(events)-1] != "done" {
				t.Fatal(events)
			}
			if len(captured) == 0 {
				t.Fatal("no request")
			}
		})
	}
}
func TestCatalogCoverageAndIsolation(t *testing.T) {
	if len(Providers()) != 42 {
		t.Fatal(len(Providers()))
	}
	seen := map[API]bool{}
	for _, p := range Providers() {
		models := Models(p)
		if len(models) == 0 {
			t.Fatal(p)
		}
		for _, m := range models {
			if m.Provider != p || m.ID == "" || m.API == "" {
				t.Fatalf("invalid %s model", p)
			}
			if m.Type == "chat" {
				seen[m.API] = true
			}
		}
	}
	for _, api := range APIs() {
		if !seen[api] {
			t.Error("missing", api)
		}
	}
	first := Models("openai")
	first[0].Name = "mutated"
	if Models("openai")[0].Name == "mutated" {
		t.Fatal("mutable global catalog")
	}
	if Models("../openai") != nil {
		t.Fatal("catalog traversal")
	}
}
func TestStreamRejectsPartialAndMalformedToolArguments(t *testing.T) {
	for _, raw := range []string{`{"x":`, `[]`, `null`, `1`, `true`, `"string"`} {
		t.Run(raw, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				event := map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "call", "function": map[string]any{"name": "echo", "arguments": raw}}}}, "finish_reason": "tool_calls"}}}
				b, _ := json.Marshal(event)
				fmt.Fprintf(w, "data: %s\n\n", b)
			}))
			defer server.Close()
			_, err := (&Client{}).Complete(context.Background(), Model{ID: "x", API: OpenAICompletions, BaseURL: server.URL}, Context{}, Options{})
			if err == nil {
				t.Fatal("accepted malformed arguments")
			}
		})
	}
	for _, api := range []API{OpenAICompletions, OpenAIResponses, AnthropicMessages, GoogleGenerative} {
		t.Run(string(api), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "data: {}\n\n") }))
			defer server.Close()
			_, err := (&Client{}).Complete(context.Background(), Model{ID: "x", API: api, BaseURL: server.URL}, Context{}, Options{})
			if err == nil {
				t.Fatal("accepted unterminated stream")
			}
		})
	}
}
func TestRetryCancellationAndBounds(t *testing.T) {
	var attempts atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 3 {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()
	m := Model{ID: "x", API: OpenAICompletions, BaseURL: server.URL}
	if _, err := (&Client{}).Complete(context.Background(), m, Context{}, Options{MaxRetries: 2}); err != nil || attempts.Load() != 3 {
		t.Fatal(err, attempts.Load())
	}
	if _, err := (&Client{}).Complete(context.Background(), m, Context{}, Options{MaxResponseBytes: 8}); err == nil {
		t.Fatal("ignored body limit")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&Client{}).Complete(ctx, m, Context{}, Options{}); ErrorCode(err) != "GENERATION_CANCELLED" {
		t.Fatal(err)
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.Copy(io.Discard, r.Body); <-r.Context().Done() }))
	defer slow.Close()
	m.BaseURL = slow.URL
	if _, err := (&Client{}).Complete(context.Background(), m, Context{}, Options{HeaderTimeout: 10 * time.Millisecond}); ErrorCode(err) != "PROVIDER_UNAVAILABLE" {
		t.Fatal(err)
	}
}
func TestDeterministicSystemUpdates(t *testing.T) {
	a, b := "one", "two"
	in := Context{Tools: []Tool{{Name: "x"}}, Messages: []Message{{Role: "system", ToolsRemoved: []string{"x"}}, {Role: "system", ToolsAdded: []Tool{{Name: "x"}}, Sections: map[string]*string{"z": &b, "a": &a}}, {Role: "user", Content: []Content{Text("hello")}}}}
	want := Normalize(in)
	for range 50 {
		if !reflect.DeepEqual(want, Normalize(in)) {
			t.Fatal("nondeterministic")
		}
	}
	if len(want.Tools) != 1 || strings.Count(want.SystemPrompt, "one") != 1 {
		t.Fatal(want)
	}
}
func TestEventStreamChecksums(t *testing.T) {
	b := awsFrame("messageStop", `{"stopReason":"stop"}`)
	if err := readEventStream(bytes.NewReader(b), func(string, []byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	b[len(b)-1] ^= 1
	if readEventStream(bytes.NewReader(b), func(string, []byte) error { return nil }) == nil {
		t.Fatal("CRC ignored")
	}
}
