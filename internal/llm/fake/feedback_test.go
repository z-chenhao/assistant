package fake

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/llm"
)

func TestFeedbackUsesTextNotRowID(t *testing.T) {
	for _, tc := range []struct{ text, category, severity string }{
		{"页面打不开，影响全公司今天投放", "service_outage", "critical"},
		{"扣款两次，订单只有一份", "billing", "high"},
		{"希望增加深色模式", "feature_request", "low"},
		{"不好用", "insufficient_evidence", "unknown"},
	} {
		raw, _ := json.Marshal(map[string]string{"row_id": "arbitrary-id", "text": tc.text})
		result := feedbackReply("Classify feedback: "+string(raw), false)
		var value map[string]string
		if json.Unmarshal([]byte(result.Text), &value) != nil || value["category"] != tc.category || value["severity"] != tc.severity || value["row_id"] != "arbitrary-id" {
			t.Fatal(result)
		}
	}
}

func TestFeedbackJourneyOriginalInputsAndDelivery(t *testing.T) {
	input := "逐条判断问题类型和严重度，再给总体结论，每条能回查。\nFeedback assets:\n" +
		`[{"name":"feedback_100.csv","mediaType":"text/csv","text":"row_id,text,attachment_ref\nF099,不好用,\n"}]`
	request := llm.GenerationRequest{Messages: []llm.Message{{Role: "user", Text: input}}}
	created := coreReply(request)
	if len(created.ToolCalls) != 1 || created.ToolCalls[0].Name != "builtin__task__manage" || !strings.Contains(created.ToolCalls[0].Arguments, "不好用") {
		t.Fatal(created)
	}
	request.Messages = append([]llm.Message{{Role: "system", Text: "general Worker executing"}}, request.Messages...)
	root := coreReply(request)
	if len(root.ToolCalls) != 1 || root.ToolCalls[0].Name != "builtin__codemode__execute" || strings.Contains(root.ToolCalls[0].Arguments, "ASSETS") || strings.Contains(root.ToolCalls[0].Arguments, "call_key") {
		t.Fatal(root)
	}
	request.Messages = []llm.Message{{Role: "user", Text: input}, {Role: "user", Origin: "runtime", Text: "Runtime task update:\n" +
		`{"details":{"task_ref":"task:feedback","kind":"completed","artifact_refs":["artifact:csv"]}}`}}
	published := coreReply(request)
	if len(published.ToolCalls) != 1 || published.ToolCalls[0].Name != "builtin__artifact__manage" || !strings.Contains(published.ToolCalls[0].Arguments, "artifact:csv") {
		t.Fatal(published)
	}
	request.Messages = append(request.Messages, llm.Message{Role: "tool", Text: "published"})
	if result := coreReply(request); len(result.ToolCalls) != 0 || !strings.Contains(result.Text, "待确认") {
		t.Fatal(result)
	}
}
