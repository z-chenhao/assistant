package fake

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/task"
)

// A follow-up contains historical user text. Only input after its runtime notice
// is fresh steering; accepted notices remain in context on later model steps.
func hasAppendedUser(messages []llm.Message) bool {
	user, notice := -1, -1
	for i, m := range messages {
		if m.Origin == "runtime" {
			notice = i
		} else if m.Role == "user" && m.Origin == "" {
			user = i
		}
	}
	return notice >= 0 && user > notice
}

// tasksReply is an explicitly selected scripted policy, never a production
// provider. Protocol assertions and state readback, not prose, are its evidence.
func tasksReply(r llm.GenerationRequest) (llm.GenerationResult, error) {
	root, followup := false, false
	for _, m := range r.Messages {
		root = root || strings.Contains(m.Text, "general Worker executing")
		followup = followup || m.Origin == "runtime"
	}
	last := r.Messages[len(r.Messages)-1]
	call := func(name string, value any) llm.GenerationResult {
		raw, _ := json.Marshal(value)
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "fixture-call", Name: name, Arguments: string(raw)}}}
	}
	if root {
		for _, m := range r.Messages {
			if strings.Contains(m.Text, "Fail worker:") {
				return llm.GenerationResult{}, llm.ModelNotFound
			}
		}
		if last.Role == "tool" {
			return llm.GenerationResult{Text: "Worker result: " + last.Text}, nil
		}
		return call("mcp__fixture__echo", struct {
			Text string `json:"text"`
		}{last.Text}), nil
	}
	if followup && !hasAppendedUser(r.Messages) {
		return llm.GenerationResult{Text: "Main follow-up: the delegated work has returned."}, nil
	}
	user := ""
	for _, m := range r.Messages {
		if m.Role == "user" && m.Origin == "" {
			user = m.Text
		}
	}
	if last.Role == "tool" {
		var prior llm.ToolCall
		for _, m := range r.Messages {
			if len(m.ToolCalls) > 0 {
				prior = m.ToolCalls[len(m.ToolCalls)-1]
			}
		}
		if prior.Name == task.SearchName {
			var results []task.Summary
			if json.Unmarshal([]byte(last.Text), &results) != nil {
				return llm.GenerationResult{Text: "Search was not available."}, nil
			}
			if strings.HasPrefix(user, "Pause: ") || strings.HasPrefix(user, "Cancel: ") {
				if len(results) != 1 {
					return llm.GenerationResult{Text: "Please clarify which work you mean."}, nil
				}
				op := "pause"
				if strings.HasPrefix(user, "Cancel: ") {
					op = "cancel"
				}
				return call(task.ManageName, task.ManageRequest{Operation: op, TaskRef: results[0].TaskRef}), nil
			}
			if strings.HasPrefix(user, "Continue: ") {
				if len(results) != 1 {
					return llm.GenerationResult{Text: fmt.Sprintf("Please clarify which of the %d matching works you mean.", len(results))}, nil
				}
				pieces := strings.SplitN(strings.TrimPrefix(user, "Continue: "), " | ", 2)
				instruction := "继续"
				if len(pieces) == 2 {
					instruction = pieces[1]
				}
				return call(task.ManageName, task.ManageRequest{Operation: "append", TaskRef: results[0].TaskRef, Instruction: instruction}), nil
			}
			return llm.GenerationResult{Text: fmt.Sprintf("Found %d matching works.", len(results))}, nil
		}
		return llm.GenerationResult{Text: "Main accepted the work."}, nil
	}
	switch {
	case strings.HasPrefix(user, "Delegate: "), strings.HasPrefix(user, "Duplicate: "):
		brief := strings.TrimPrefix(strings.TrimPrefix(user, "Delegate: "), "Duplicate: ")
		result := call(task.ManageName, task.ManageRequest{Operation: "create", Task: brief})
		if strings.HasPrefix(user, "Duplicate: ") {
			second := result.ToolCalls[0]
			second.ID = "duplicate-call"
			result.ToolCalls = append(result.ToolCalls, second)
		}
		return result, nil
	case strings.HasPrefix(user, "Delegate many: "):
		result := llm.GenerationResult{}
		for i, brief := range strings.Split(strings.TrimPrefix(user, "Delegate many: "), " | ") {
			c := call(task.ManageName, task.ManageRequest{Operation: "create", Task: brief}).ToolCalls[0]
			c.ID = fmt.Sprint(i)
			result.ToolCalls = append(result.ToolCalls, c)
		}
		return result, nil
	case strings.HasPrefix(user, "Find: "):
		return call(task.SearchName, task.SearchRequest{Query: strings.TrimPrefix(user, "Find: ")}), nil
	case strings.HasPrefix(user, "Continue: "):
		query := strings.SplitN(strings.TrimPrefix(user, "Continue: "), " | ", 2)[0]
		return call(task.SearchName, task.SearchRequest{Query: query}), nil
	case strings.HasPrefix(user, "Pause: "), strings.HasPrefix(user, "Cancel: "):
		return call(task.SearchName, task.SearchRequest{Query: strings.TrimPrefix(strings.TrimPrefix(user, "Pause: "), "Cancel: ")}), nil
	default:
		return llm.GenerationResult{Text: "Main synchronous answer."}, nil
	}
}
