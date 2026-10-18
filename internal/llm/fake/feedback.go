package fake

import (
	"encoding/json"
	"strings"

	"github.com/z-chenhao/erii/internal/llm"
)

// Deterministic language classifier, deliberately independent of the hidden
// row/Gold mapping. This proves plumbing and exact evidence, not model quality.
func feedbackReply(input string, continued bool) llm.GenerationResult {
	var row struct {
		ID         string `json:"row_id"`
		Text       string `json:"text"`
		Attachment string `json:"attachment_ref"`
		Hold       bool   `json:"hold"`
	}
	if json.Unmarshal([]byte(strings.TrimPrefix(input, "Classify feedback: ")), &row) != nil {
		return llm.GenerationResult{Text: "Invalid feedback fixture"}
	}
	if row.Hold && !continued {
		raw, _ := json.Marshal(map[string]string{"target": "feedback-checkpoint", "text": row.ID})
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "checkpoint", Name: "mcp__fixture__send", Arguments: string(raw)}}}
	}
	category, severity := "insufficient_evidence", "unknown"
	for _, rule := range []struct{ clue, category string }{{"程序错误", "product_bug"}, {"扣款", "billing"}, {"希望增加", "feature_request"}, {"操作步骤", "usability"}, {"服务中断", "service_outage"}, {"页面打不开", "service_outage"}} {
		if strings.Contains(row.Text, rule.clue) {
			category = rule.category
			break
		}
	}
	if category != "insufficient_evidence" {
		severity = "medium"
		switch {
		case strings.Contains(row.Text, "全公司") || strings.Contains(row.Text, "紧急"):
			severity = "critical"
		case strings.Contains(row.Text, "两次") || strings.Contains(row.Text, "严重"):
			severity = "high"
		case strings.Contains(row.Text, "轻微") || row.Text == "希望增加深色模式":
			severity = "low"
		}
	}
	raw, _ := json.Marshal(map[string]string{"row_id": row.ID, "category": category, "severity": severity, "evidence": row.Text, "attachment_ref": row.Attachment})
	return llm.GenerationResult{Text: string(raw)}
}

// Independent review Worker reads current source; no hidden expected verdict.
func cancellationReview(req llm.GenerationRequest) llm.GenerationResult {
	count := 0
	last := ""
	for _, m := range req.Messages {
		if m.Role == "tool" {
			count++
			last = m.Text
		}
	}
	paths := []string{"AGENTS.md", "internal/run/service.go"}
	if count < len(paths) {
		raw, _ := json.Marshal(map[string]string{"path": paths[count]})
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "review", Name: "builtin__workspace__read", Arguments: string(raw)}}}
	}
	if strings.Contains(last, "tree[i].Parent == parent") && !strings.Contains(last, "Cancel(tree,") {
		return llm.GenerationResult{Text: "BLOCKED: internal/run/service.go Cancel visits only direct children; grandchild accepts a late result. Recurse into descendants and run TestGrandchildCancel."}
	}
	return llm.GenerationResult{Text: "Review found no direct-child-only cancellation pattern."}
}
