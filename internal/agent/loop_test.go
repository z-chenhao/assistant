package agent

import (
	"testing"

	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/llm"
)

func TestRuntimeNormalizationAndProviderBoundary(t *testing.T) {
	call := &conversation.ToolCall{ID: "provider-id", Name: "builtin__text__echo", Arguments: `{"text":"ok"}`}
	parts := []conversation.ContentPart{{Type: "text", Text: "reason"}, {Type: "tool_call", ToolCall: call}}
	normalized, err := conversation.Normalize(parts)
	if err != nil {
		t.Fatal(err)
	}
	call.Name = "mutated"
	if normalized[1].ToolCall.Name != "builtin__text__echo" {
		t.Fatal("mutable caller content leaked")
	}
	messages, err := ProviderMessages([]AgentMessage{TextMessage("system", "rules"), {Type: "assistant", Content: normalized}, {Type: "tool_result", ToolCallID: "provider-id", Content: conversation.Text("ok")}})
	if err != nil || messages[0].Role != "system" || messages[1].ToolCalls[0].ID != "provider-id" || messages[2].ToolCallID != "provider-id" {
		t.Fatal("lost association", messages, err)
	}
	for _, message := range []AgentMessage{{Type: "custom", Content: conversation.Text("do not elevate")}, {Type: "user", Content: []conversation.ContentPart{{Type: "image", ArtifactRef: "artifact:opaque"}}}} {
		if _, err := ProviderMessages([]AgentMessage{message}); err != llm.ModelIncompatible {
			t.Fatal("unsupported provider content accepted", err)
		}
	}
	if _, err := conversation.Normalize([]conversation.ContentPart{{Type: "image", ArtifactRef: "/host/secret"}}); err == nil {
		t.Fatal("accepted raw host path")
	}
}
