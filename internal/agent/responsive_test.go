package agent

import (
	"testing"

	"github.com/z-chenhao/assistant/internal/conversation"
)

func TestContextDeferredOperationIDsKeepBatchAndLegacyCorrelation(t *testing.T) {
	// Provider call IDs can repeat across independent generation batches.
	for _, old := range []bool{false, true} {
		ref := "0-0"
		if old {
			ref = "operation:0:0"
		}
		messages := []AgentMessage{
			{Type: "tool_result", ToolCallID: "reused", Content: conversation.Text(`{"status":"pending","operation_ref":"` + ref + `"}`)},
			{Type: "tool_result", ToolCallID: "reused", Content: conversation.Text(`{"status":"pending","operation_ref":"1-0"}`)},
		}
		if got := deferredOperationRef(messages, 0, 0, "reused"); got != ref {
			t.Fatal("later batch replaced the saved correlation", got, ref)
		}
		if got := deferredOperationRef(messages, 1, 0, "reused"); got != "1-0" {
			t.Fatal("earlier batch replaced the new correlation", got)
		}
	}
}
