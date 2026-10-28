package fake

import (
	"testing"

	"github.com/z-chenhao/erii/internal/agent"
	contextbuilder "github.com/z-chenhao/erii/internal/context"
	"github.com/z-chenhao/erii/internal/llm"
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
	wire, e := agent.ProviderMessages([]agent.AgentMessage{state})
	if e != nil {
		t.Fatal(e)
	}
	request := llm.GenerationRequest{Messages: append([]llm.Message{{Role: "user", Text: `Capabilities: [{"name":"echo","arguments":{}},{"name":"echo","arguments":{}}]`}}, wire...)}
	next := capabilitiesReply(request)
	if len(next.ToolCalls) != 1 || next.ToolCalls[0].ID != "capability:1" {
		t.Fatal("summary replay repeated a completed fixture operation", next)
	}
}
