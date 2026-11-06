package provider

import (
	"encoding/json"
	"testing"
)

func TestWebAccessNativeAccountWireAndNamespace(t *testing.T) {
	model := Model{ID: "frozen", API: OpenAIResponses, Provider: "steward", BaseURL: "https://api.openai.com/v1", Compat: map[string]any{"supportsMaxOutputTokens": false}}
	wire, e := BuildRequest(model, Context{WebSearch: true, WebSearchDomains: []string{"w3.org"}, Messages: []Message{{Role: "user", Content: []Content{Text("query")}}}}, Options{AccessToken: "synthetic-token"})
	if e != nil {
		t.Fatal(e)
	}
	encoded, _ := json.Marshal(wire.Body)
	var body map[string]any
	if json.Unmarshal(encoded, &body) != nil {
		t.Fatal("wire JSON")
	}
	if body["max_tool_calls"] != nil || body["max_output_tokens"] != nil || body["tool_choice"] != "required" {
		t.Fatal(body)
	}
	tool := body["tools"].([]any)[0].(map[string]any)
	if tool["filters"].(map[string]any)["allowed_domains"].([]any)[0] != "w3.org" {
		t.Fatal(tool)
	}
	namespace := responsesTools([]Tool{{Name: "example", Namespace: "steward", Parameters: json.RawMessage(`{"type":"object"}`)}})[0].(map[string]any)
	if namespace["type"] != "namespace" || namespace["description"] != "Steward tools" {
		t.Fatal("namespace description required by account API", namespace)
	}
}
