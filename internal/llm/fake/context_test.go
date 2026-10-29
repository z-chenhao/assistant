package fake

import (
	"testing"

	"github.com/z-chenhao/collie/internal/agent"
	contextbuilder "github.com/z-chenhao/collie/internal/context"
	"github.com/z-chenhao/collie/internal/llm"
)

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
