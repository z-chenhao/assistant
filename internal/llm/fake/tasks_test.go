package fake

import (
	"encoding/json"
	"testing"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/task"
)

func TestTaskFixtureAppendedControlOnFollowup(t *testing.T) {
	for _, input := range []string{"Cancel: Batch hold", "Pause: Batch hold", "Continue: Batch hold", "Complete: Batch hold"} {
		t.Run(input, func(t *testing.T) {
			req := llm.GenerationRequest{Messages: []llm.Message{
				{Role: "system", Text: "Main conversation"},
				{Role: "user", Origin: "runtime", Text: "Runtime task update: Completed independent work"},
				{Role: "user", Text: input},
			}}
			result, err := childrenReply(req)
			if err != nil || len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != task.SearchName {
				t.Fatal("fresh user control was replaced by a completion reply", result, err)
			}
			var query task.SearchRequest
			if json.Unmarshal([]byte(result.ToolCalls[0].Arguments), &query) != nil || query.Query != "Batch hold" {
				t.Fatal("wrong work selected", result)
			}
		})
	}
}

func TestTaskFixtureFollowupWithoutFreshInput(t *testing.T) {
	result, err := childrenReply(llm.GenerationRequest{Messages: []llm.Message{
		{Role: "system", Text: "Main conversation"},
		{Role: "user", Text: "Delegate: already handled"},
		{Role: "assistant", Text: "Main accepted the work."},
		{Role: "user", Origin: "runtime", Text: "Runtime task update: Completed independent work"},
	}})
	if err != nil || len(result.ToolCalls) != 0 || result.Text != "Main follow-up: the delegated work has returned." {
		t.Fatal(result, err)
	}
}

func TestBatchFixtureAppendedCancelOnFollowup(t *testing.T) {
	result, err := childrenReply(llm.GenerationRequest{Messages: []llm.Message{
		{Role: "system", Text: "Main conversation"},
		{Role: "user", Origin: "runtime", Text: "Runtime task update\n{\"content\":[{\"text\":\"Batch aggregate: 98 results, 2 failed.\"}]}"},
		{Role: "assistant", Text: "Main follow-up: earlier accepted reply"},
		{Role: "user", Text: "Cancel: Batch hold"},
	}})
	if err != nil || len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != task.SearchName {
		t.Fatal("batch completion hid fresh cancellation", result, err)
	}
}
