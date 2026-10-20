package fake

import (
	"encoding/json"
	"testing"
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
