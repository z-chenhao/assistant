package fake

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/z-chenhao/erii/internal/llm"
)

func TestWeddingFixturePreservesSourceAndDoesNotConfuseAnchorWithIntent(t *testing.T) {
	source, err := os.ReadFile("../../../evals/fixtures/gc08-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var compact any
	if json.Unmarshal(source, &compact) != nil {
		t.Fatal("invalid fixture")
	}
	raw, _ := json.Marshal(compact)
	anchor := "Task anchor: 婚礼准备 Wedding source: {truncated title\n婚礼准备\nWedding source: " + string(raw) + "\n婚礼准备现在最应该处理什么？"
	for _, tc := range []struct {
		instruction string
		guests      any
		draft       bool
	}{
		{"", nil, false}, {"宾客人数已经定了，128 人。", float64(128), false},
		{"给酒店写封邮件确认桌次，但发之前给我看。", float64(128), true},
	} {
		r := llm.GenerationRequest{Messages: []llm.Message{{Role: "system", Text: "general Worker executing"}, {Role: "user", Text: anchor}}}
		if tc.instruction != "" {
			r.Messages = append(r.Messages, llm.Message{Role: "user", Text: "Previous Task instruction: 宾客人数已经定了，128 人。"}, llm.Message{Role: "user", Text: "Task instruction: " + tc.instruction + "\nArtifact references: []"})
		}
		result := weddingReply(r)
		if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "builtin__artifact__manage" {
			t.Fatal(result)
		}
		var saved struct{ Text string }
		if json.Unmarshal([]byte(result.ToolCalls[0].Arguments), &saved) != nil {
			t.Fatal(result)
		}
		var doc struct {
			Facts     map[string]any
			Draft     map[string]any
			Checklist []map[string]any
		}
		if json.Unmarshal([]byte(saved.Text), &doc) != nil || doc.Facts["guests"] != tc.guests || doc.Facts["table_count"] != nil || doc.Facts["per_table"] != nil || (doc.Draft != nil) != tc.draft {
			t.Fatal(saved.Text)
		}
		if tc.guests != nil && (doc.Checklist[0]["status"] != "completed" || doc.Checklist[1]["status"] != "active") {
			t.Fatal(doc)
		}
		r.Messages = append(r.Messages, llm.Message{Role: "assistant", ToolCalls: result.ToolCalls}, llm.Message{Role: "tool", Text: `{"artifact_refs":["artifact:checklist"]}`})
		publish := weddingReply(r)
		if len(publish.ToolCalls) != 1 || publish.ToolCalls[0].Name != "builtin__artifact__manage" {
			t.Fatal(publish)
		}
		r.Messages = append(r.Messages, llm.Message{Role: "assistant", ToolCalls: publish.ToolCalls}, llm.Message{Role: "tool", Text: "Final artifacts selected."})
		next := weddingReply(r)
		if tc.draft {
			if len(next.ToolCalls) != 1 || next.ToolCalls[0].Name != "mcp__wedding__send" {
				t.Fatal(next)
			}
			var action map[string]any
			_ = json.Unmarshal([]byte(next.ToolCalls[0].Arguments), &action)
			if action["recipient"] != "banquet@example.test" || len(action["attachments"].([]any)) != 0 {
				t.Fatal(action)
			}
		} else if len(next.ToolCalls) != 0 {
			t.Fatal("premature external call", next)
		}
	}
}

func TestWeddingFixtureDoesNotInferGuestsFromSourceNoise(t *testing.T) {
	anchor := "Task anchor: wedding\nWedding source: {\"contacts\":{\"hotel\":\"banquet@example.test\"},\"unrelated_note\":\"128 chairs\"}\n"
	r := llm.GenerationRequest{Messages: []llm.Message{{Role: "system", Text: "general Worker executing"}, {Role: "user", Text: anchor + "给酒店写封邮件，先给我看。"}}}
	if result := weddingReply(r); len(result.ToolCalls) != 0 || result.Text != "请先确认宾客人数；未生成含猜测人数的邮件，未联系酒店。" {
		t.Fatal("guessed guests", result)
	}
	r.Messages[1].Text = anchor + "婚礼准备现在最应该处理什么？"
	result := weddingReply(r)
	var saved struct{ Text string }
	if len(result.ToolCalls) != 1 || json.Unmarshal([]byte(result.ToolCalls[0].Arguments), &saved) != nil {
		t.Fatal(result)
	}
	var doc struct{ Facts map[string]any }
	if json.Unmarshal([]byte(saved.Text), &doc) != nil || doc.Facts["guests"] != nil {
		t.Fatal("source noise became a Task fact", saved)
	}
}
