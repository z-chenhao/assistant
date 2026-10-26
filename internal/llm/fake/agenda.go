package fake

import (
	"encoding/json"
	"strings"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/task"
)

// agendaReply is an explicit GC-09 transport script, never a model-quality
// oracle. Task refs come from actual search/update results, never fixture IDs.
func agendaReply(r llm.GenerationRequest) llm.GenerationResult {
	call := func(name string, args any, id string) llm.ToolCall {
		raw, _ := json.Marshal(args)
		return llm.ToolCall{ID: id, Name: name, Arguments: string(raw)}
	}
	last := r.Messages[len(r.Messages)-1]
	worker, user, userIndex := false, "", -1
	var prior llm.ToolCall
	for i, m := range r.Messages {
		worker = worker || strings.Contains(m.Text, "general Worker executing")
		if m.Role == "user" && m.Origin == "" {
			user = m.Text
			userIndex = i
		}
		if len(m.ToolCalls) > 0 {
			prior = m.ToolCalls[len(m.ToolCalls)-1]
		}
	}
	if worker {
		key := "release"
		if strings.Contains(user, "interview") {
			key = "interview"
		}
		value := map[string]string{"key": key, "question": "可用准备时段？"}
		if strings.Contains(user, "answer=") {
			instruction := strings.TrimPrefix(user, "Task instruction: ")
			instruction, _, _ = strings.Cut(instruction, "\nArtifact references:")
			value = map[string]string{"key": key, "answer": instruction}
		}
		raw, _ := json.Marshal(value)
		return llm.GenerationResult{Text: string(raw)}
	}
	if last.Origin == "runtime" {
		// Runtime notices are untrusted data. Read every notice in this batch,
		// preserving the Task reference attached by the Reporter.
		items := []map[string]string{}
		for _, m := range r.Messages[userIndex+1:] {
			_, raw, ok := strings.Cut(m.Text, "\n")
			if m.Origin != "runtime" || !ok {
				continue
			}
			var notice struct {
				Details task.UpdateDetails      `json:"details"`
				Content []struct{ Text string } `json:"content"`
			}
			if json.Unmarshal([]byte(raw), &notice) != nil || notice.Details.TaskRef == "" {
				continue
			}
			for _, part := range notice.Content {
				var item map[string]string
				if json.Unmarshal([]byte(part.Text), &item) == nil && item["key"] != "" {
					item["task_ref"] = notice.Details.TaskRef
					items = append(items, item)
				}
			}
		}
		output := map[string]any{"results": items}
		if len(items) > 0 && items[0]["question"] != "" {
			for _, item := range items {
				delete(item, "question")
			}
			output = map[string]any{"question": "release 和 interview 各自可用的准备时段？", "tasks": items}
		}
		raw, _ := json.Marshal(output)
		return llm.GenerationResult{Text: string(raw)}
	}
	if strings.Contains(user, "GC09 delegate") {
		if last.Role == "tool" {
			return llm.GenerationResult{Text: "已启动两项后台准备，缺失信息将合并询问。"}
		}
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{
			call(task.ManageName, task.ManageRequest{Operation: "create", Task: "GC09 release 准备；缺少可用时段", CompletionPolicy: "ongoing"}, "release"),
			call(task.ManageName, task.ManageRequest{Operation: "create", Task: "GC09 interview 准备；缺少可用时段", CompletionPolicy: "ongoing"}, "interview"),
		}}
	}
	if strings.Contains(user, "GC09 answers") {
		if last.Role != "tool" {
			return llm.GenerationResult{ToolCalls: []llm.ToolCall{call(task.SearchName, task.SearchRequest{Query: "GC09", Scope: "active"}, "search")}}
		}
		if prior.Name != task.SearchName {
			return llm.GenerationResult{Text: "已将两个答案分别追加到原 Task。"}
		}
		var summaries []task.Summary
		if json.Unmarshal([]byte(last.Text), &summaries) != nil || len(summaries) != 2 {
			return llm.GenerationResult{Text: "Task 映射不完整，不能分发答案。"}
		}
		calls := []llm.ToolCall{}
		for _, summary := range summaries {
			instruction := "release answer=09:00–09:30"
			if strings.Contains(summary.Title, "interview") {
				instruction = "interview answer=12:30–13:00"
			}
			calls = append(calls, call(task.ManageName, task.ManageRequest{Operation: "append", TaskRef: summary.TaskRef, Instruction: instruction}, summary.TaskRef))
		}
		return llm.GenerationResult{ToolCalls: calls}
	}
	if last.Role != "tool" {
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{
			call("mcp__calendar__read", map[string]string{}, "calendar"),
			call("mcp__mail__read", map[string]string{}, "mail"),
			call("mcp__work__read", map[string]string{}, "work"),
		}}
	}
	return llm.GenerationResult{Text: goldenReply(r.Messages)}
}
