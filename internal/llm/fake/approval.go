package fake

import (
	"encoding/json"
	"strings"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/task"
)

func approvalReply(r llm.GenerationRequest) llm.GenerationResult {
	last := r.Messages[len(r.Messages)-1]
	root, followup := false, false
	user := ""
	for _, m := range r.Messages {
		root = root || strings.Contains(m.Text, "general Worker executing")
		followup = followup || m.Origin == "runtime" && strings.HasPrefix(m.Text, "Runtime task update")
		if m.Role == "user" && m.Origin == "" {
			user = m.Text
		}
	}
	if followup && !hasAppendedUser(r.Messages) {
		return llm.GenerationResult{Text: "Observed Worker update: " + last.Text}
	}
	if !root && (strings.HasPrefix(user, "Pause: ") || strings.HasPrefix(user, "Cancel: ")) {
		result, _ := tasksReply(r)
		return result
	}
	if last.Role == "tool" {
		return llm.GenerationResult{Text: "Observed effect result: " + last.Text}
	}
	args := last.Text
	name := "mcp__fixture__send"
	if root {
		const marker = "Effect fixture:\n"
		if at := strings.LastIndex(args, marker); at >= 0 {
			args = args[at+len(marker):]
		}
	} else if strings.HasPrefix(args, "Delegate effect: ") {
		raw, _ := json.Marshal(task.ManageRequest{Operation: "create", Task: "Effect fixture:\n" + strings.TrimPrefix(args, "Delegate effect: ")})
		args = string(raw)
		name = task.ManageName
	}
	return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "effect", Name: name, Arguments: args}}}
}
