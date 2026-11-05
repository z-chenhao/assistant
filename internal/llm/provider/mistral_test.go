package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"testing"
)

func TestMistralNativeFieldsAndToolReplay(t *testing.T) {
	input := Context{SystemPrompt: "Keep evidence.", Tools: []Tool{{Name: "echo", Parameters: json.RawMessage(`{"type":"object"}`)}}, Messages: []Message{
		{Role: "assistant", Content: []Content{{Type: "thinking", Thinking: "Earlier reasoning."}, {Type: "toolCall", ID: "123456789", Name: "echo", Arguments: map[string]any{}}, {Type: "toolCall", ID: "call_123456789", Name: "echo", Arguments: map[string]any{}}}},
		{Role: "toolResult", ToolCallID: "123456789", ToolName: "echo", Content: []Content{Text("first")}},
		{Role: "toolResult", ToolCallID: "call_123456789", ToolName: "echo", Content: []Content{Text("second")}},
	}}
	before, _ := json.Marshal(input)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("invalid request")
		}
		if r.URL.Path != "/v1/chat/completions" || body["max_tokens"] != float64(77) || body["reasoning_effort"] != "max" || body["prompt_cache_key"] != "session" || r.Header.Get("Authorization") != "Bearer key" {
			t.Error("incorrect native request", r.URL, body)
		}
		for _, unsupported := range []string{"store", "stream_options", "max_completion_tokens", "prompt_mode"} {
			if _, exists := body[unsupported]; exists {
				t.Error("OpenAI-specific parameter sent to Mistral", unsupported)
			}
		}
		messages := list(body["messages"])
		calls := list(record(messages[1])["tool_calls"])
		one, two := str(record(calls[0])["id"]), str(record(calls[1])["id"])
		valid := regexp.MustCompile(`^[a-zA-Z0-9]{9}$`)
		if !valid.MatchString(one) || !valid.MatchString(two) || one == two || record(messages[2])["tool_call_id"] != one || record(messages[3])["tool_call_id"] != two {
			t.Error("call/result identities diverged", messages)
		}
		if record(list(record(messages[1])["content"])[0])["type"] != "thinking" || record(messages[2])["name"] != "echo" {
			t.Error("native reasoning or result name lost", messages)
		}
		fmt.Fprint(w, "data: {\"id\":\"response-mistral\",\"choices\":[{\"delta\":{\"content\":[{\"type\":\"thinking\",\"thinking\":[{\"type\":\"text\",\"text\":\"Checked.\"}]}]}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Done.\"},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()
	model := Model{ID: "mistral", API: MistralConversations, BaseURL: server.URL, Reasoning: true, ThinkingLevelMap: map[string]any{"high": "max"}}
	result, err := (&Client{}).Complete(context.Background(), model, input, Options{APIKey: "key", MaxTokens: 77, Reasoning: "high", SessionID: "session"})
	if err != nil || result.ResponseID != "response-mistral" || len(result.Content) != 2 || result.Content[0].Thinking != "Checked." || result.Content[1].Text != "Done." {
		t.Fatal(result, err)
	}
	after, _ := json.Marshal(input)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("caller transcript was mutated")
	}
}

func TestResponsesTokenLimitKeepsPartialOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Partial answer\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"response.incomplete\",\"response\":{\"id\":\"partial\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"usage\":{\"input_tokens\":4,\"output_tokens\":2}}}\n\n")
	}))
	defer server.Close()
	result, err := (&Client{}).Complete(context.Background(), Model{ID: "model", API: OpenAIResponses, BaseURL: server.URL}, Context{}, Options{})
	if err != nil || result.StopReason != "length" || len(result.Content) != 1 || result.Content[0].Text != "Partial answer" || result.Usage.Output != 2 {
		t.Fatal("valid token truncation became a provider failure", result, err)
	}
}
