package fake

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/task"
)

func researchCorpus(t *testing.T) []researchDocument {
	t.Helper()
	raw, err := os.ReadFile("../../../evals/fixtures/gc16-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var source struct{ Documents []researchDocument }
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	return source.Documents
}

func TestResearchScriptUsesMainTaskAndOneParallelChildExchange(t *testing.T) {
	r := llm.GenerationRequest{Messages: []llm.Message{{Role: "user", Text: "找出最早公开提出 Anchor Replay 的项目，并确认后来有没有改变定义。只用一手来源。"}}}
	main := researchReply(r)
	if len(main.ToolCalls) != 1 || main.ToolCalls[0].Name != task.ManageName {
		t.Fatalf("Main bypassed Task: %+v", main)
	}
	r.Messages = append([]llm.Message{{Role: "system", Text: "general Worker executing"}}, llm.Message{Role: "user", Text: "Task anchor: research"})
	root := researchReply(r)
	if len(root.ToolCalls) != 2 || root.ToolCalls[0].ID == root.ToolCalls[1].ID {
		t.Fatalf("missing independent paths: %+v", root)
	}
	for _, c := range root.ToolCalls {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal([]byte(c.Arguments), &fields)
		if c.Name != agent.RunToolName || len(fields) != 1 || fields["task"] == nil {
			t.Fatalf("expanded Child request: %+v", c)
		}
	}
}

func TestResearchChildWalksSecondaryLeadThenEveryOfficialLink(t *testing.T) {
	r := llm.GenerationRequest{Messages: []llm.Message{{Role: "system", Text: "general Worker executing"}, {Role: "user", Text: "核对 Anchor Replay 时间线"}}}
	appendResult := func(result llm.GenerationResult, value any) {
		raw, _ := json.Marshal(value)
		r.Messages = append(r.Messages, llm.Message{Role: "assistant", ToolCalls: result.ToolCalls}, llm.Message{Role: "tool", Text: string(raw)})
	}
	first := researchReply(r)
	if first.ToolCalls[0].Name != "mcp__research__search" {
		t.Fatal(first)
	}
	appendResult(first, map[string]any{"hits": []map[string]string{{"id": "D2"}}})
	docs := researchCorpus(t)
	for _, index := range []int{1, 0, 2, 3} {
		next := researchReply(r)
		if len(next.ToolCalls) != 1 || next.ToolCalls[0].Name != "mcp__research__open" || !strings.Contains(next.ToolCalls[0].Arguments, docs[index].ID) {
			t.Fatalf("stopped at a lead: %+v", next)
		}
		appendResult(next, docs[index])
	}
	final := researchReply(r)
	var found []researchDocument
	if len(final.ToolCalls) != 0 || json.Unmarshal([]byte(final.Text), &found) != nil || len(found) != 4 {
		t.Fatalf("incomplete final sources: %+v", final)
	}
}

func TestResearchReportFailsClosedWithoutBothVerifiedPaths(t *testing.T) {
	docs := researchCorpus(t)
	if _, ok := researchReport(docs); !ok {
		t.Fatal("complete corpus rejected")
	}
	for index := range docs {
		partial := append([]researchDocument{}, docs[:index]...)
		partial = append(partial, docs[index+1:]...)
		if _, ok := researchReport(partial); ok {
			t.Fatalf("missing %s accepted", docs[index].ID)
		}
		changed := append([]researchDocument{}, docs...)
		changed[index].Claim = "unsupported guess"
		if _, ok := researchReport(changed); ok {
			t.Fatalf("unsupported %s claim accepted", docs[index].ID)
		}
	}
	docs[0].Type = "independent_article"
	if _, ok := researchReport(docs); ok {
		t.Fatal("secondary date accepted as primary")
	}
}
