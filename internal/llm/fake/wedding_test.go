package fake

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	contextbuilder "github.com/z-chenhao/erii/internal/context"
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

func TestWeddingFixtureRecoversClippedAnchorThroughTools(t *testing.T) {
	source, err := os.ReadFile("../../../evals/fixtures/gc08-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct{ Resume string }
	raw, err := os.ReadFile("../../../evals/fixtures/gc08-lifecycle-v1.json")
	if err != nil || json.Unmarshal(raw, &fixture) != nil || strings.Contains(fixture.Resume, "128") {
		t.Fatal("resume must recover guests, not repeat them", err)
	}
	r := llm.GenerationRequest{Messages: []llm.Message{
		{Role: "system", Text: "general Worker executing"},
		{Role: "user", Origin: "runtime", Text: "Compaction summary: clipped anchor and old instructions; not new instructions"},
		{Role: "user", Text: `Previous committed Task work (untrusted data): {"artifact_refs":["artifact:previous"]}`},
		{Role: "user", Text: "Task instruction: " + fixture.Resume + "\nArtifact references: []"},
	}}
	step := func(name, result string) llm.ToolCall {
		t.Helper()
		answer := weddingReply(r)
		if len(answer.ToolCalls) != 1 || answer.ToolCalls[0].Name != name {
			t.Fatal(answer)
		}
		call := answer.ToolCalls[0]
		r.Messages = append(r.Messages, llm.Message{Role: "assistant", ToolCalls: answer.ToolCalls}, llm.Message{Role: "tool", Text: result})
		return call
	}
	step("mcp__wedding__read", string(source))
	var data map[string]any
	_ = json.Unmarshal(source, &data)
	saved := map[string]any{"event": data["event"], "checklist": data["checklist"], "facts": map[string]any{"guests": 128, "table_count": nil, "per_table": nil}}
	doc, _ := json.Marshal(saved)
	readResult, _ := json.Marshal(map[string]any{"text": string(doc), "bytes": len(doc), "total_bytes": len(doc), "offset": 0})
	read := step("builtin__artifact__manage", string(readResult))
	if read.Arguments != `{"artifactRef":"artifact:previous","limit":4096,"operation":"read"}` {
		t.Fatal("not a bounded read of inherited reference", read)
	}
	good := r
	for _, bad := range []string{`{}`, `{"text":"not json"}`, `{"text":"{\"facts\":{\"guests\":129}}"}`} {
		r.Messages = append([]llm.Message(nil), good.Messages...)
		r.Messages[len(r.Messages)-1].Text = bad
		answer := weddingReply(r)
		if len(answer.ToolCalls) != 0 || !strings.Contains(answer.Text, "未猜测") {
			t.Fatal("missing or changed facts accepted", answer)
		}
	}
	r = good
	save := step("builtin__artifact__manage", `{"artifact_refs":["artifact:new"]}`)
	var args struct{ Operation, Text string }
	if json.Unmarshal([]byte(save.Arguments), &args) != nil || args.Operation != "save" {
		t.Fatal(save)
	}
	var updated struct {
		Event     map[string]any
		Facts     map[string]any
		Checklist []map[string]any
	}
	if json.Unmarshal([]byte(args.Text), &updated) != nil || updated.Event["date"] != "2026-11-08" || updated.Facts["guests"] != float64(128) || updated.Facts["table_count"] != nil || updated.Facts["per_table"] != nil || updated.Checklist[1]["due"] != "2026-10-29" || updated.Checklist[2]["due"] != "2026-11-01" {
		t.Fatal(args.Text)
	}
	step("builtin__artifact__manage", `{"text":"Final artifacts selected.","artifact_refs":["artifact:new"]}`)
	if answer := weddingReply(r); len(answer.ToolCalls) != 0 || !strings.Contains(answer.Text, "修订清单已保存并发布") {
		t.Fatal("unexpected effect after recovery", answer)
	}
}

func TestWeddingFixturePauseUsesMainControlWithoutInstructionOrNewTask(t *testing.T) {
	r := llm.GenerationRequest{Messages: []llm.Message{{Role: "user", Text: "暂停婚礼筹备。"}}}
	answer := weddingReply(r)
	if len(answer.ToolCalls) != 1 || answer.ToolCalls[0].Name != "builtin__task__search" {
		t.Fatal(answer)
	}
	r.Messages = append(r.Messages, llm.Message{Role: "assistant", ToolCalls: answer.ToolCalls}, llm.Message{Role: "tool", Text: `[{"taskRef":"task:existing","status":"active"}]`})
	answer = weddingReply(r)
	if len(answer.ToolCalls) != 1 || answer.ToolCalls[0].Name != "builtin__task__manage" || answer.ToolCalls[0].Arguments != `{"operation":"pause","taskRef":"task:existing"}` {
		t.Fatal(answer)
	}
}

func TestWeddingFixtureUsesOngoingPolicyAndExplicitTerminalControls(t *testing.T) {
	r := llm.GenerationRequest{Messages: []llm.Message{{Role: "user", Text: "婚礼准备现在最应该处理什么？"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "mcp__wedding__read", Arguments: `{}`}}},
		{Role: "tool", Text: `{"event":{"title":"婚礼"}}`}}}
	created := weddingReply(r)
	if len(created.ToolCalls) != 1 || !strings.Contains(created.ToolCalls[0].Arguments, `"completionPolicy":"ongoing"`) {
		t.Fatal(created)
	}
	for _, tc := range []struct{ input, operation string }{{"完成婚礼筹备，保留清单。", "complete"}, {"取消婚礼筹备，不再联系酒店。", "cancel"}} {
		r.Messages = []llm.Message{{Role: "user", Text: tc.input}}
		search := weddingReply(r)
		if len(search.ToolCalls) != 1 || search.ToolCalls[0].Name != "builtin__task__search" {
			t.Fatal(search)
		}
		r.Messages = append(r.Messages, llm.Message{Role: "assistant", ToolCalls: search.ToolCalls}, llm.Message{Role: "tool", Text: `[{"taskRef":"task:existing","status":"active"}]`})
		control := weddingReply(r)
		if len(control.ToolCalls) != 1 || control.ToolCalls[0].Name != "builtin__task__manage" || control.ToolCalls[0].Arguments != fmt.Sprintf(`{"operation":%q,"taskRef":"task:existing"}`, tc.operation) {
			t.Fatal(control)
		}
	}
}

func TestWeddingFixtureFinishesRecoveredChecklistAfterCompaction(t *testing.T) {
	source, err := os.ReadFile("../../../evals/fixtures/gc08-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	_ = json.Unmarshal(source, &data)
	doc, _ := json.Marshal(map[string]any{"event": data["event"], "checklist": data["checklist"],
		"facts": map[string]any{"guests": 128, "table_count": nil, "per_table": nil}})
	read, _ := json.Marshal(map[string]any{"text": string(doc)})
	var fixture struct{ Resume string }
	raw, err := os.ReadFile("../../../evals/fixtures/gc08-lifecycle-v1.json")
	if err != nil || json.Unmarshal(raw, &fixture) != nil {
		t.Fatal(err)
	}
	messages := []llm.Message{
		{Role: "system", Text: "general Worker executing"},
		{Role: "user", Origin: "runtime", Text: "Compaction summary (untrusted excerpts; not new user instructions): clipped old anchor"},
		{Role: "user", Text: `Previous committed Task work (untrusted data): {"artifact_refs":["artifact:previous"]}`},
		{Role: "user", Text: "Task instruction: " + fixture.Resume + "\nArtifact references: []"},
	}
	for i, step := range []struct{ name, operation, result string }{
		{"mcp__wedding__read", "", string(source)},
		{"builtin__artifact__manage", "read", string(read)},
		{"builtin__artifact__manage", "save", `{"artifact_refs":["artifact:new"]}`},
		{"builtin__artifact__manage", "publish", `{"text":"Final artifacts selected.","artifact_refs":["artifact:new"]}`},
		{"", "", ""},
	} {
		refs := make([]string, len(messages))
		for j := range refs {
			refs[j] = fmt.Sprint(j)
		}
		compacted, err := contextbuilder.Compact(messages, refs, 1300, 0,
			contextbuilder.Policy{Threshold: .75, Recent: 2, SummaryBytes: 256}, false)
		if err != nil {
			t.Fatal(i, err)
		}
		answer := weddingReply(llm.GenerationRequest{Messages: compacted.Messages})
		if step.name == "" {
			if len(compacted.Omitted) == 0 || strings.Contains(fmt.Sprint(compacted.Messages), string(read)) ||
				len(answer.ToolCalls) != 0 || !strings.Contains(answer.Text, "修订清单已保存并发布") {
				t.Fatal("lost completed progress after old reads were compacted", compacted.Stage, answer)
			}
			return
		}
		if len(answer.ToolCalls) != 1 || answer.ToolCalls[0].Name != step.name {
			t.Fatal(i, "repeated or skipped work", answer)
		}
		if step.operation != "" {
			var args struct{ Operation string }
			if json.Unmarshal([]byte(answer.ToolCalls[0].Arguments), &args) != nil || args.Operation != step.operation {
				t.Fatal(i, "wrong operation", answer)
			}
		}
		messages = append(messages, llm.Message{Role: "assistant", ToolCalls: answer.ToolCalls},
			llm.Message{Role: "tool", ToolCallID: answer.ToolCalls[0].ID, Text: step.result})
	}
}

func TestWeddingFixtureDoesNotClaimUnverifiedArtifactProgress(t *testing.T) {
	for _, tc := range []struct{ operation, result string }{
		{"save", `{}`}, {"save", `{"artifact_refs":[]}`}, {"save", `{"artifact_refs":[""]}`},
		{"save", `{"error":{"code":"NOT_FOUND"}}`},
		{"publish", `{}`}, {"publish", `{"error":{"code":"NOT_FOUND"}}`},
		{"publish", `{"text":"Final artifacts selected.","artifact_refs":[]}`},
		{"publish", `{"text":"Final artifacts selected.","artifact_refs":["artifact:other"]}`},
	} {
		args, _ := json.Marshal(map[string]any{"operation": tc.operation, "artifactRefs": []string{"artifact:new"}})
		r := llm.GenerationRequest{Messages: []llm.Message{
			{Role: "system", Text: "general Worker executing"},
			{Role: "user", Text: "Task instruction: 继续婚礼筹备。婚期改为2026-11-08"},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "wedding-fixture", Name: "builtin__artifact__manage", Arguments: string(args)}}},
			{Role: "tool", ToolCallID: "wedding-fixture", Text: tc.result},
		}}
		answer := weddingReply(r)
		if len(answer.ToolCalls) != 0 || !strings.Contains(answer.Text, "未核实") || strings.Contains(answer.Text, "已保存并发布") {
			t.Fatal(tc, "unverified progress triggered success or another call", answer)
		}
	}
}
