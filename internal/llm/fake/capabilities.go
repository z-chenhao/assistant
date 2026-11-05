package fake

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/task"
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
		if m.Role == "user" && strings.Contains(m.Text, "Review cancellation") {
			if hasTool(request.Tools, "builtin__read") {
				return cancellationReview(request)
			}
			if lastMessage.Role == "tool" || lastMessage.Origin == "runtime" {
				return llm.GenerationResult{Text: "Observed review Task: " + lastMessage.Text}
			}
			raw, _ := json.Marshal(task.ManageRequest{Operation: "create", Task: "Review cancellation"})
			return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "review-task", Name: task.ManageName, Arguments: string(raw)}}}
		}
	}
	if lastMessage.Role == "user" && lastMessage.Origin != "runtime" && lastMessage.Text == "Responsive MCP ping" {
		return llm.GenerationResult{Text: "RESPONSIVE_MCP_PONG"}
	}
	if lastMessage.Origin == "runtime" {
		text := lastMessage.Text
		if strings.HasPrefix(text, "Runtime task update") {
			var update struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			}
			_, payload, _ := strings.Cut(text, "\n")
			if json.Unmarshal([]byte(payload), &update) == nil && len(update.Content) > 0 {
				parts := []string{}
				for _, part := range update.Content {
					parts = append(parts, part.Text)
				}
				// Preserve literal result text for the live UI/scroll scenario.
				text = strings.Join(parts, "\n")
			}
		}
		return llm.GenerationResult{Text: "Observed Worker update: " + text}
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
	// A summarized Worker update also finishes an automatically delegated plan.
	// Without this receipt a fresh Reporter Main would recreate the same Task.
	if hasTool(request.Tools, task.ManageName) {
		for _, m := range request.Messages[start+1:] {
			if contextSummary(m).TaskUpdate {
				return llm.GenerationResult{Text: summarizedTaskUpdate(m)}
			}
		}
	}
	if strings.HasPrefix(source, "Delegate capabilities: ") {
		if lastMessage.Role == "tool" {
			return llm.GenerationResult{Text: "Observed Task result: " + lastMessage.Text}
		}
		for _, m := range request.Messages[start+1:] {
			if contextSummary(m).TaskUpdate {
				return llm.GenerationResult{Text: summarizedTaskUpdate(m)}
			}
			for _, id := range contextReceipts(m) {
				if id == "delegate" {
					return llm.GenerationResult{Text: "Observed Task result: completed delegation receipt retained in working context."}
				}
			}
		}
		raw := capabilityTask(request, strings.TrimPrefix(source, "Delegate capabilities: "))
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "delegate", Name: "builtin__task_manage", Arguments: string(raw)}}}
	}
	var plan []struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
		Progress  string          `json:"progress,omitempty"`
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
			if _, ok := strings.CutPrefix(id, "discover:"); ok {
				continue
			}
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
	// The fixture model follows the same role boundary as a real model: missing
	// Worker-only tools are delegated through Task, never injected into Main.
	workerOnly := func(name string) bool {
		return !hasTool(request.Tools, name) && (strings.HasPrefix(name, "mcp__") && !hasTool(request.Tools, "builtin__codemode_execute") || name == "builtin__codemode_execute" || name == "builtin__read" || name == "builtin__bash" || name == "builtin__edit" || name == "builtin__write" || name == "builtin__skill_list" || name == "builtin__skill_view")
	}
	if !strings.HasPrefix(source, "Direct Capabilities: ") && hasTool(request.Tools, task.ManageName) && workerOnly(p.Name) {
		// Consecutive execution steps share one Task so file registrations and
		// other Task-scoped resources survive between Code Mode invocations.
		end := next + 1
		for end < len(plan) && workerOnly(plan[end].Name) {
			end++
		}
		raw, _ := json.Marshal(plan[next:end])
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "capability:" + strconv.Itoa(end-1), Name: task.ManageName, Arguments: string(capabilityTask(request, string(raw)))}}}
	}
	if !strings.HasPrefix(source, "Direct Capabilities: ") && strings.HasPrefix(p.Name, "mcp__") && !hasTool(request.Tools, p.Name) && hasTool(request.Tools, "builtin__codemode_execute") {
		raw, _ := json.Marshal(map[string]string{"source": "return await tools." + p.Name + "(" + string(p.Arguments) + ");"})
		return llm.GenerationResult{Text: p.Progress, ToolCalls: []llm.ToolCall{{ID: "capability:" + strconv.Itoa(next), Name: "builtin__codemode_execute", Arguments: string(raw)}}}
	}
	return llm.GenerationResult{Text: p.Progress, ToolCalls: []llm.ToolCall{{ID: "capability:" + strconv.Itoa(next), Name: p.Name, Arguments: string(p.Arguments)}}}
}

var capabilityArtifact = regexp.MustCompile(`artifact:[a-zA-Z0-9_-]+`)

func summarizedTaskUpdate(m llm.Message) string {
	if contextSummary(m).UnknownEffect {
		return "Observed Worker update: UNKNOWN_SIDE_EFFECT retained in working context; do not repeat the action."
	}
	return "Observed Worker update: Task update retained in working context."
}

func capabilityTask(request llm.GenerationRequest, plan string) []byte {
	refs := []string{}
	seen := map[string]bool{}
	add := func(ref string) {
		if !seen[ref] {
			seen[ref] = true
			refs = append(refs, ref)
		}
	}
	for _, ref := range capabilityArtifact.FindAllString(plan, -1) {
		add(ref)
	}
	for _, m := range request.Messages {
		for _, ref := range m.ImageRefs {
			add(ref)
		}
	}
	raw, _ := json.Marshal(task.ManageRequest{Operation: "create", Task: "Capabilities fixture\nCapabilities: " + plan, ArtifactRefs: refs})
	return raw
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
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "review", Name: "builtin__read", Arguments: string(raw)}}}
	}
	if strings.Contains(last, "tree[i].Parent == parent") && !strings.Contains(last, "Cancel(tree,") {
		return llm.GenerationResult{Text: "BLOCKED: internal/run/service.go Cancel visits only direct children; grandchild accepts a late result. Recurse into descendants and run TestGrandchildCancel."}
	}
	return llm.GenerationResult{Text: "Review found no direct-child-only cancellation pattern."}
}
