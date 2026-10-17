package fake

import (
	"encoding/json"
	"strings"

	"github.com/z-chenhao/erii/internal/llm"
)

// Explicit scripted model fixture. Results still come from the real registry,
// executor, scoped files and stdio transport, never from canned success text.
func capabilitiesReply(request llm.GenerationRequest) llm.GenerationResult {
	lastMessage := request.Messages[len(request.Messages)-1]
	if lastMessage.Origin == "runtime" {
		return llm.GenerationResult{Text: "Observed Worker update: " + lastMessage.Text}
	}
	var source string
	start := 0
	for i := len(request.Messages) - 1; i >= 0; i-- {
		m := request.Messages[i]
		if m.Role == "user" && (strings.Contains(m.Text, "Capabilities: ") || strings.Contains(m.Text, "Delegate capabilities: ")) {
			source = m.Text
			start = i
			break
		}
	}
	if strings.HasPrefix(source, "Delegate capabilities: ") {
		if lastMessage.Role == "tool" {
			return llm.GenerationResult{Text: "Observed Task result: " + lastMessage.Text}
		}
		raw, _ := json.Marshal(map[string]string{"operation": "create", "task": "Capabilities fixture\nCapabilities: " + strings.TrimPrefix(source, "Delegate capabilities: ")})
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "delegate", Name: "builtin__task__manage", Arguments: string(raw)}}}
	}
	if i := strings.LastIndex(source, "Capabilities: "); i >= 0 {
		source = source[i+len("Capabilities: "):]
	} else {
		return llm.GenerationResult{Text: "No scripted capability request."}
	}
	var plan []struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal([]byte(source), &plan) != nil {
		return llm.GenerationResult{Text: "Invalid fixture plan."}
	}
	count := 0
	last := ""
	for _, m := range request.Messages[start+1:] {
		if m.Role == "tool" {
			count++
			last = m.Text
		}
	}
	if count >= len(plan) {
		return llm.GenerationResult{Text: "Observed Tool result: " + last}
	}
	p := plan[count]
	return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "capability", Name: p.Name, Arguments: string(p.Arguments)}}}
}
