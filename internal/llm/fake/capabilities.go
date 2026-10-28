package fake

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/z-chenhao/erii/internal/llm"
)

// Explicit scripted model fixture. Results still come from the real registry,
// executor, scoped files and stdio transport, never from canned success text.
func capabilitiesReply(request llm.GenerationRequest) llm.GenerationResult {
	lastMessage := request.Messages[len(request.Messages)-1]
	// Automatic recall is still present in the captured request. Scripted Tool
	// plans use the latest semantic message, not the untrusted retrieval suffix.
	for i := len(request.Messages) - 1; i >= 0; i-- {
		m := request.Messages[i]
		if m.Origin == "runtime" && strings.HasPrefix(m.Text, "Retrieved durable context") {
			continue
		}
		if m.Source != nil && m.Source.Kind == "working_state" {
			continue
		}
		lastMessage = m
		break
	}
	for _, m := range request.Messages {
		if m.Role == "user" && m.Text == "Review cancellation" {
			return cancellationReview(request)
		}
	}
	if lastMessage.Origin == "runtime" {
		return llm.GenerationResult{Text: "Observed Worker update: " + lastMessage.Text}
	}
	var source string
	start := 0
	for i := len(request.Messages) - 1; i >= 0; i-- {
		m := request.Messages[i]
		if m.Role == "user" && m.Origin != "runtime" && (strings.Contains(m.Text, "Capabilities: ") || strings.Contains(m.Text, "Delegate capabilities: ")) {
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
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "delegate", Name: "builtin__task_manage", Arguments: string(raw)}}}
	}
	var plan []struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	found := false
	// A Task title may repeat a truncated plan, and JavaScript may itself
	// contain a Child request marker. Select the complete JSON plan only.
	for offset := 0; offset < len(source); {
		i := strings.Index(source[offset:], "Capabilities: ")
		if i < 0 {
			break
		}
		offset += i + len("Capabilities: ")
		if json.NewDecoder(strings.NewReader(source[offset:])).Decode(&plan) == nil {
			found = true
			break
		}
	}
	if !found {
		return llm.GenerationResult{Text: "Invalid fixture plan."}
	}
	next := 0
	last := ""
	for _, m := range request.Messages[start+1:] {
		ids := contextReceipts(m)
		if m.Role == "tool" {
			ids = append(ids, m.ToolCallID)
			last = m.Text
		}
		for _, id := range ids {
			// Compaction may discard any older completed exchange. Keep plan
			// progress in this Fake provider's call identity, which survives in
			// the retained result, rather than count the remaining messages.
			ordinal, ok := strings.CutPrefix(id, "capability:")
			i, err := strconv.Atoi(ordinal)
			if !ok || err != nil || i < 0 || i >= len(plan) {
				return llm.GenerationResult{Text: "Invalid fixture receipt."}
			}
			next = max(next, i+1)
		}
	}
	if next >= len(plan) {
		return llm.GenerationResult{Text: "Observed Tool result: " + last}
	}
	p := plan[next]
	return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "capability:" + strconv.Itoa(next), Name: p.Name, Arguments: string(p.Arguments)}}}
}

func cancellationReview(req llm.GenerationRequest) llm.GenerationResult {
	count := 0
	last := ""
	for _, m := range req.Messages {
		if m.Role == "tool" {
			count++
			last = m.Text
		}
	}
	paths := []string{"AGENTS.md", "internal/run/service.go"}
	if count < len(paths) {
		raw, _ := json.Marshal(map[string]string{"path": paths[count]})
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "review", Name: "builtin__workspace_read", Arguments: string(raw)}}}
	}
	if strings.Contains(last, "tree[i].Parent == parent") && !strings.Contains(last, "Cancel(tree,") {
		return llm.GenerationResult{Text: "BLOCKED: internal/run/service.go Cancel visits only direct children; grandchild accepts a late result. Recurse into descendants and run TestGrandchildCancel."}
	}
	return llm.GenerationResult{Text: "Review found no direct-child-only cancellation pattern."}
}
