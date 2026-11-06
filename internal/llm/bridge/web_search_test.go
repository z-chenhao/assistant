package bridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/llm"
)

type searchTransport func(*http.Request) (*http.Response, error)

func (f searchTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHostedSearchRequiresNativeEvidence(t *testing.T) {
	for _, scenario := range []struct{ native, pinned bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
		native := scenario.native
		t.Run(map[bool]string{false: "prose rejected", true: "search and sources accepted"}[native]+map[bool]string{false: " legacy", true: " Pi payload"}[scenario.pinned], func(t *testing.T) {
			var out bytes.Buffer
			calls := 0
			server := Server{out: &out, HTTPClient: &http.Client{Transport: searchTransport(func(req *http.Request) (*http.Response, error) {
				body := `{"models":[{"slug":"frozen-model","visibility":"list"}]}`
				header := http.Header{"Content-Type": {"application/json"}}
				if req.URL.Path == "/v1/responses" {
					calls++
					if req.Header.Get("Authorization") != "Bearer fixture-token" {
						t.Fatal("account credential not preserved")
					}
					var wire map[string]any
					if json.NewDecoder(req.Body).Decode(&wire) != nil {
						t.Fatal("payload")
					}
					if wire["model"] != "frozen-model" || wire["tool_choice"] != "required" || wire["store"] != false || wire["max_tool_calls"] != nil {
						t.Fatal(wire)
					}
					tools := wire["tools"].([]any)
					if len(tools) != 1 || tools[0].(map[string]any)["type"] != "web_search" {
						t.Fatal(wire)
					}
					if scenario.pinned && tools[0].(map[string]any)["filters"].(map[string]any)["allowed_domains"].([]any)[0] != "example.com" {
						t.Fatal("pinned filter lost", wire)
					}
					raw, _ := json.Marshal(wire)
					if strings.Contains(string(raw), "private history") {
						t.Fatal("history leaked")
					}
					body = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"answer\"}\n\n"
					if native {
						body += "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"web_search_call\",\"status\":\"completed\",\"action\":{\"sources\":[{\"type\":\"url\",\"url\":\"https://example.com/\",\"title\":\"Example\"}]}}}\n\n"
					}
					body += "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"search\",\"usage\":{\"input_tokens\":7,\"output_tokens\":2,\"total_tokens\":9}}}\n\n"
					header.Set("Content-Type", "text/event-stream")
				}
				return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
			})}}
			request := llm.GenerationRequest{GenerationID: "search", RunID: "run", SessionID: "run", Snapshot: llm.Snapshot{SnapshotID: "snapshot", ProviderKind: "openai_account", WireAPI: "openai_responses", ModelID: "frozen-model", MaxOutputTokens: 1024, ContextWindow: 8192, Compatibility: llm.Compatibility{ToolResultFormat: "openai"}}, WebSearch: &llm.WebSearchRequest{InvocationID: "tool", Query: "query"}, Messages: []llm.Message{{Role: "system", Text: "Search"}, {Role: "user", Text: "query"}}}
			if scenario.pinned {
				request.WebSearch.Payload = json.RawMessage(`{"model":"frozen-model","instructions":"Search","input":[{"role":"user","content":[{"type":"input_text","text":"query"}]}],"tools":[{"type":"web_search","filters":{"allowed_domains":["example.com"]}}],"include":["web_search_call.action.sources"],"store":false,"stream":true,"tool_choice":"required","parallel_tool_calls":true}`)
			}
			server.generate(context.Background(), generationRequest{GenerationRequest: request, Connection: llm.Connection{Endpoint: apiURL, AccessToken: "fixture-token"}}, &job{})
			scanner := bufio.NewScanner(&out)
			var last llm.GenerationEvent
			for scanner.Scan() {
				var frame struct {
					Params llm.GenerationEvent `json:"params"`
				}
				if json.Unmarshal(scanner.Bytes(), &frame) != nil {
					t.Fatal(scanner.Text())
				}
				last = frame.Params
			}
			if calls != 1 {
				t.Fatal(calls)
			}
			if native {
				if last.Kind != "completed" || last.WebSearch == nil || len(last.WebSearch.Sources) != 1 || last.Usage == nil || last.Usage.InputTokens != 7 {
					t.Fatal(last)
				}
			} else if last.Kind != "error" || last.ErrorCode != "INVALID_PROVIDER_RESPONSE" {
				t.Fatal(last)
			}
		})
	}
}

func TestSearchCompletionBounds(t *testing.T) {
	s := newEventStream("search", 1024)
	s.accept(llm.GenerationEvent{Kind: "completed", Sequence: 1, WebSearch: &llm.WebSearchResult{Sources: make([]llm.WebSource, 21)}})
	if d := <-s.events; d.err != llm.InvalidProviderResponse {
		t.Fatal(d)
	}
}

func TestHostedSearchPinnedPayloadRejectsConversationContinuation(t *testing.T) {
	var out bytes.Buffer
	dispatched := false
	server := Server{out: &out, HTTPClient: &http.Client{Transport: searchTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/v1/responses" {
			dispatched = true
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"models":[{"slug":"frozen-model","visibility":"list"}]}`)), Request: req}, nil
	})}}
	request := llm.GenerationRequest{GenerationID: "search", RunID: "run", SessionID: "run", Snapshot: llm.Snapshot{SnapshotID: "snapshot", ProviderKind: "openai_account", WireAPI: "openai_responses", ModelID: "frozen-model", MaxOutputTokens: 1024, ContextWindow: 8192, Compatibility: llm.Compatibility{ToolResultFormat: "openai"}}, WebSearch: &llm.WebSearchRequest{InvocationID: "tool", Query: "query", Payload: json.RawMessage(`{"model":"frozen-model","instructions":"Search","input":[{"role":"user","content":[{"type":"input_text","text":"query"}]}],"tools":[{"type":"web_search"}],"store":false,"stream":true,"tool_choice":"required","previous_response_id":"private-conversation"}`)}, Messages: []llm.Message{{Role: "system", Text: "Search"}, {Role: "user", Text: "query"}}}
	server.generate(context.Background(), generationRequest{GenerationRequest: request, Connection: llm.Connection{Endpoint: apiURL, AccessToken: "fixture-token"}}, &job{})
	if dispatched || !strings.Contains(out.String(), `"error_code":"INVALID_PROVIDER_RESPONSE"`) {
		t.Fatal("unsafe payload accepted", out.String())
	}
}
