package fake

import (
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/task"
)

//go:embed feedback.js
var feedbackSource string

// coreReply keeps GC-19/20's existing scripts while driving GC-17 from only
// the original question and allowed CSV/attachment bytes, never hidden Gold.
func coreReply(r llm.GenerationRequest) llm.GenerationResult {
	const marker = "Feedback assets:\n"
	user, worker := "", false
	var update struct {
		Details task.UpdateDetails `json:"details"`
	}
	for _, m := range r.Messages {
		if m.Role == "user" && strings.Contains(m.Text, marker) {
			user = m.Text
		}
		worker = worker || strings.Contains(m.Text, "general Worker executing")
		if m.Origin == "runtime" {
			_, raw, ok := strings.Cut(m.Text, "\n")
			if ok {
				_ = json.Unmarshal([]byte(raw), &update)
			}
		}
	}
	if user == "" {
		return capabilitiesReply(r)
	}
	call := func(name string, args any) llm.GenerationResult {
		raw, _ := json.Marshal(args)
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "feedback-journey", Name: name, Arguments: string(raw)}}}
	}
	last := r.Messages[len(r.Messages)-1]
	if !worker {
		if update.Details.TaskRef != "" {
			if last.Role != "tool" {
				return call("builtin__artifact__manage", map[string]any{"operation": "publish", "artifactRefs": update.Details.ArtifactRefs})
			}
			return llm.GenerationResult{Text: "100 条反馈已逐项分析；分类、严重度和 row_id 回查见 CSV。资料不足的样本保留待确认。"}
		}
		if last.Role == "tool" {
			return llm.GenerationResult{Text: "已开始逐行分析反馈，完成后交付可回查的 CSV 与汇总。"}
		}
		return call(task.ManageName, task.ManageRequest{Operation: "create", Task: user})
	}
	if last.Role == "tool" {
		return llm.GenerationResult{Text: "反馈分析汇总（资料不足保留待确认）：" + last.Text}
	}
	_, assets, _ := strings.Cut(user, marker)
	if !json.Valid([]byte(assets)) {
		return llm.GenerationResult{Text: "反馈资料无效，不能声称完成。"}
	}
	return call("builtin__codemode__execute", map[string]string{"source": strings.Replace(feedbackSource, "ASSETS", assets, 1)})
}

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
