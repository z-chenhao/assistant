package fake

import (
	"fmt"
	"strings"
	"testing"

	contextbuilder "github.com/z-chenhao/erii/internal/context"
	"github.com/z-chenhao/erii/internal/llm"
)

func TestCapabilityPlanProgressSurvivesCompactionAndIdenticalCalls(t *testing.T) {
	// GC-11 hit this after the merged Tool schema grew. Counting retained Tool
	// messages repeats the last step forever once compaction removes step 0.
	// Identical neighboring calls also rule out matching only name/arguments.
	r := llm.GenerationRequest{Messages: []llm.Message{{Role: "user", Text: `Capabilities: [{"name":"mcp__fixture__echo","arguments":{"text":"first"}},{"name":"mcp__fixture__echo","arguments":{"text":"same"}},{"name":"mcp__fixture__echo","arguments":{"text":"same"}}]`}}}
	omitted := false
	for i := 0; i < 3; i++ {
		result := capabilitiesReply(r)
		if len(result.ToolCalls) != 1 {
			t.Fatal("plan ended early", i, result)
		}
		call := result.ToolCalls[0]
		r.Messages = append(r.Messages, llm.Message{Role: "assistant", ToolCalls: result.ToolCalls},
			llm.Message{Role: "tool", ToolCallID: call.ID, Text: strings.Repeat("source observation ", 60)})
		refs := make([]string, len(r.Messages))
		for j := range refs {
			refs[j] = fmt.Sprint(j)
		}
		compacted, err := contextbuilder.Compact(r.Messages, refs, 900, 0, contextbuilder.Policy{Threshold: .75, Recent: 2, SummaryBytes: 256}, false)
		if err != nil {
			t.Fatal(err)
		}
		omitted = omitted || len(compacted.Omitted) > 0
		r.Messages = compacted.Messages
	}
	result := capabilitiesReply(r)
	if !omitted || len(result.ToolCalls) != 0 || !strings.HasPrefix(result.Text, "Observed Tool result:") {
		t.Fatal("compacted completed plan repeated a call", omitted, result)
	}
}

func TestCapabilityPlanRejectsInvalidReceiptOrdinals(t *testing.T) {
	for _, id := range []string{"", "capability", "other:0", "capability:-1", "capability:1", "capability:x"} {
		r := llm.GenerationRequest{Messages: []llm.Message{
			{Role: "user", Text: `Capabilities: [{"name":"mcp__fixture__echo","arguments":{"text":"only"}}]`},
			{Role: "tool", ToolCallID: id, Text: "completed"},
		}}
		result := capabilitiesReply(r)
		if len(result.ToolCalls) != 0 || result.Text != "Invalid fixture receipt." {
			t.Fatal("invalid receipt advanced the plan", id, result)
		}
	}
}
