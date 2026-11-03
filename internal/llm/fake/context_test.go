package fake

import (
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/agent"
	contextbuilder "github.com/z-chenhao/assistant/internal/context"
	"github.com/z-chenhao/assistant/internal/llm"
)

func TestDelegateKeepsCompletedReceiptAcrossSummary(t *testing.T) {
	testDelegateSummary(t, []llm.Message{{Role: "tool", ToolCallID: "delegate", Text: `{"task_ref":"task:accepted"}`}}, "Observed Task result:")
}

func TestDelegateKeepsTaskUpdateAcrossSummary(t *testing.T) {
	state := agent.RuntimeContext("task_update", "task:accepted", false, map[string]string{"status": "completed"})
	wire, err := agent.ProviderMessages([]agent.AgentMessage{state}, false)
	if err != nil {
		t.Fatal(err)
	}
	testDelegateSummary(t, wire, "Observed Worker update:")
}

func testDelegateSummary(t *testing.T, sources []llm.Message, observed string) {
	t.Helper()
	input := llm.Message{Role: "user", Text: `Delegate capabilities: [{"name":"builtin__codemode_execute","arguments":{"source":"return await agent.run({task:\"Capabilities: []\"});"}}]`}
	first := capabilitiesReply(llm.GenerationRequest{Messages: []llm.Message{input}})
	if len(first.ToolCalls) != 1 || first.ToolCalls[0].ID != "delegate" {
		t.Fatal("missing initial delegation", first)
	}
	for round := 0; round < 2; round++ {
		messages, _, err := contextbuilder.SummaryRequest(sources, nil, 6000, 2048)
		if err != nil {
			t.Fatal(err)
		}
		summary := contextReply(llm.GenerationRequest{Messages: messages})
		state := agent.RuntimeContext("working_state", "artifact:history", false, struct {
			Summary string `json:"summary"`
		}{summary.Text})
		wire, err := agent.ProviderMessages([]agent.AgentMessage{state}, false)
		if err != nil {
			t.Fatal(err)
		}
		request := llm.GenerationRequest{Messages: append([]llm.Message{input}, wire...)}
		result := capabilitiesReply(request)
		if len(result.ToolCalls) != 0 || !strings.HasPrefix(result.Text, observed) {
			t.Fatal("compacted delegation repeated Task create", round, result)
		}
		// An older input's receipt cannot consume a fresh explicit delegation.
		request.Messages = append(request.Messages, input)
		if result := capabilitiesReply(request); len(result.ToolCalls) != 1 || result.ToolCalls[0].ID != "delegate" {
			t.Fatal("old delegation receipt suppressed new input", result)
		}
		sources = wire
	}
}

func TestToolFreeSummaryRetainsFixtureReceiptProgress(t *testing.T) {
	messages, _, e := contextbuilder.SummaryRequest([]llm.Message{{Role: "tool", ToolCallID: "capability:0", Text: "completed actual tool result"}}, nil, 6000, 2048)
	if e != nil {
		t.Fatal(e)
	}
	summary := contextReply(llm.GenerationRequest{Messages: messages})
	state := agent.RuntimeContext("working_state", "artifact:history", false, struct {
		Summary string `json:"summary"`
	}{summary.Text})
	wire, e := agent.ProviderMessages([]agent.AgentMessage{state}, false)
	if e != nil {
		t.Fatal(e)
	}
	request := llm.GenerationRequest{Messages: append([]llm.Message{{Role: "user", Text: `Capabilities: [{"name":"echo","arguments":{}},{"name":"echo","arguments":{}}]`}}, wire...)}
	next := capabilitiesReply(request)
	if len(next.ToolCalls) != 1 || next.ToolCalls[0].ID != "capability:1" {
		t.Fatal("summary replay repeated a completed fixture operation", next)
	}
}

func TestArtifactReadKeepsCompletedReceiptAcrossSummary(t *testing.T) {
	messages, _, err := contextbuilder.SummaryRequest([]llm.Message{{Role: "tool", ToolCallID: "artifact-fixture", Text: "Actual read result"}}, nil, 6000, 2048)
	if err != nil {
		t.Fatal(err)
	}
	summary := contextReply(llm.GenerationRequest{Messages: messages})
	state := agent.RuntimeContext("working_state", "artifact:history", false, struct {
		Summary string `json:"summary"`
	}{summary.Text})
	wire, err := agent.ProviderMessages([]agent.AgentMessage{state}, false)
	if err != nil {
		t.Fatal(err)
	}
	request := llm.GenerationRequest{Messages: append([]llm.Message{{Role: "user", Text: `{"operation":"read","artifactRef":"artifact:input"}`}}, wire...)}
	result := artifactsReply(request)
	if len(result.ToolCalls) != 0 || result.Text == "" {
		t.Fatal("compaction repeated the completed read", result)
	}
}
