package fake

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/task"
)

func TestAgendaReadsSourcesWithoutCallingAvailableWrites(t *testing.T) {
	r := agendaReply(llm.GenerationRequest{Messages: []llm.Message{{Role: "user", Text: "我今天先做什么？"}}})
	if len(r.ToolCalls) != 3 {
		t.Fatal(r)
	}
	for i, name := range []string{"mcp__calendar__read", "mcp__mail__read", "mcp__work__read"} {
		if r.ToolCalls[i].Name != name || r.ToolCalls[i].Arguments != "{}" {
			t.Fatal(r)
		}
	}
}

func TestAgendaMergesQuestionsAndPreservesRuntimeTaskMapping(t *testing.T) {
	messages := []llm.Message{{Role: "user", Text: "GC09 delegate"}}
	for _, key := range []string{"release", "interview"} {
		raw, _ := json.Marshal(map[string]any{"details": task.UpdateDetails{TaskRef: "task:" + key}, "content": []map[string]string{{"text": `{"key":"` + key + `","question":"可用准备时段？"}`}}})
		messages = append(messages, llm.Message{Role: "user", Origin: "runtime", Text: "Runtime task update\n" + string(raw)})
	}
	r := agendaReply(llm.GenerationRequest{Messages: messages})
	var output struct {
		Question string
		Tasks    []map[string]string
	}
	if json.Unmarshal([]byte(r.Text), &output) != nil || output.Question == "" || len(output.Tasks) != 2 || len(r.ToolCalls) != 0 {
		t.Fatal(r)
	}
	for _, item := range output.Tasks {
		if item["task_ref"] != "task:"+item["key"] || item["question"] != "" {
			t.Fatal(output)
		}
	}
}

func TestAgendaAppendsDistinctAnswersOnlyAfterUnambiguousSearch(t *testing.T) {
	messages := []llm.Message{{Role: "user", Text: "GC09 answers"}, {Role: "assistant", ToolCalls: []llm.ToolCall{{Name: task.SearchName}}}}
	for _, count := range []int{1, 2, 3} {
		rows := []task.Summary{{TaskRef: "task:r", Title: "GC09 release"}}
		if count >= 2 {
			rows = append(rows, task.Summary{TaskRef: "task:i", Title: "GC09 interview"})
		}
		if count == 3 {
			rows = append(rows, task.Summary{TaskRef: "task:other"})
		}
		raw, _ := json.Marshal(rows)
		r := agendaReply(llm.GenerationRequest{Messages: append(messages, llm.Message{Role: "tool", Text: string(raw)})})
		if count != 2 {
			if len(r.ToolCalls) != 0 || !strings.Contains(r.Text, "映射不完整") {
				t.Fatal(r)
			}
			continue
		}
		if len(r.ToolCalls) != 2 {
			t.Fatal(r)
		}
		for i, call := range r.ToolCalls {
			var args task.ManageRequest
			if json.Unmarshal([]byte(call.Arguments), &args) != nil || args.Operation != "append" || args.TaskRef != rows[i].TaskRef {
				t.Fatal(call)
			}
		}
	}
}
