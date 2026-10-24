package fake

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/task"
)

// researchReply is an explicitly selected GC-16 script. Sources arrive only
// through real Tools/Child results. It proves mechanics, never model quality.
func researchReply(r llm.GenerationRequest) llm.GenerationResult {
	call := func(name string, args any) llm.GenerationResult {
		raw, _ := json.Marshal(args)
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "research-fixture", Name: name, Arguments: string(raw)}}}
	}
	worker, root, user := false, false, ""
	var prior llm.ToolCall
	var update struct {
		Details task.UpdateDetails `json:"details"`
	}
	for _, m := range r.Messages {
		worker = worker || strings.Contains(m.Text, "general Worker executing")
		root = root || strings.HasPrefix(m.Text, "Task anchor:")
		if m.Role == "user" && m.Origin == "" {
			user = m.Text
		}
		if len(m.ToolCalls) > 0 {
			prior = m.ToolCalls[len(m.ToolCalls)-1]
		}
		if m.Origin == "runtime" {
			_, raw, ok := strings.Cut(m.Text, "\n")
			if ok {
				_ = json.Unmarshal([]byte(raw), &update)
			}
		}
	}
	last := r.Messages[len(r.Messages)-1]
	if !worker {
		if update.Details.TaskRef != "" && !hasAppendedUser(r.Messages) {
			if last.Role != "tool" {
				return call("builtin__artifact__manage", map[string]any{"operation": "publish", "artifactRefs": update.Details.ArtifactRefs})
			}
			return llm.GenerationResult{Text: "研究报告已保存：Atlas 的一手时间线、改名与扩展、Aster 差异及未知项见附件。"}
		}
		if last.Role == "tool" {
			return llm.GenerationResult{Text: "已开始核对两条独立证据路径，完成后交付报告。"}
		}
		return call(task.ManageName, task.ManageRequest{Operation: "create", Task: user})
	}
	if root {
		if last.Role != "tool" {
			result := call(agent.RunToolName, agent.AgentRunRequest{Task: "核对 Anchor Replay 的一手时间线，追踪后续版本与定义变化；返回读到的来源。"})
			other := call(agent.RunToolName, agent.AgentRunRequest{Task: "独立核对 Aster Step Cache 的定义，返回读到的一手来源，用于排除同名近似技术。"}).ToolCalls[0]
			other.ID = "research-alternative"
			result.ToolCalls = append(result.ToolCalls, other)
			return result
		}
		if prior.Name == "builtin__artifact__manage" {
			var args struct{ Operation string }
			_ = json.Unmarshal([]byte(prior.Arguments), &args)
			if args.Operation == "save" {
				var output struct {
					Refs []string `json:"artifact_refs"`
				}
				if json.Unmarshal([]byte(last.Text), &output) == nil && len(output.Refs) == 1 {
					return call("builtin__artifact__manage", map[string]any{"operation": "publish", "artifactRefs": output.Refs})
				}
			}
			if args.Operation == "publish" {
				return llm.GenerationResult{Text: "已核对两条证据路径；独立报告含一手引用、事实、推断与未发现证据。"}
			}
			return llm.GenerationResult{Text: "报告保存或发布未确认，不能声称完成。"}
		}
		var docs []researchDocument
		for _, m := range r.Messages {
			if m.Role == "tool" {
				var child agent.AgentResult
				var found []researchDocument
				if json.Unmarshal([]byte(m.Text), &child) == nil && json.Unmarshal([]byte(child.Text), &found) == nil {
					docs = append(docs, found...)
				}
			}
		}
		report, ok := researchReport(docs)
		if !ok {
			return llm.GenerationResult{Text: "来源不完整或定义无法核实，未发布确定结论。"}
		}
		return call("builtin__artifact__manage", map[string]string{"operation": "save", "name": "anchor-replay-research.md", "mediaType": "text/markdown", "text": report})
	}
	// A Child walks discovered references rather than receiving a canned answer
	// or Parent context. D2 is a lead; later official pages require more hops.
	var docs []researchDocument
	var next []string
	seen := map[string]bool{}
	for _, m := range r.Messages {
		if m.Role != "tool" {
			continue
		}
		var search struct{ Hits []struct{ ID string } }
		if json.Unmarshal([]byte(m.Text), &search) == nil {
			for _, hit := range search.Hits {
				next = append(next, hit.ID)
			}
		}
		var doc researchDocument
		if json.Unmarshal([]byte(m.Text), &doc) == nil && doc.ID != "" {
			docs = append(docs, doc)
			seen[doc.ID] = true
			next = append(next, doc.Links...)
		}
	}
	if len(next) == 0 && len(docs) == 0 && last.Role != "tool" {
		query := "Anchor Replay"
		if strings.Contains(user, "Aster") {
			query = "Aster Step Cache"
		}
		return call("mcp__research__search", map[string]string{"key": query})
	}
	for _, id := range next {
		if !seen[id] {
			return call("mcp__research__open", map[string]string{"key": id})
		}
	}
	raw, _ := json.Marshal(docs)
	return llm.GenerationResult{Text: string(raw)}
}

type researchDocument struct {
	ID    string   `json:"id"`
	Type  string   `json:"type"`
	Date  string   `json:"date"`
	Title string   `json:"title"`
	Claim string   `json:"claim"`
	URL   string   `json:"url"`
	Links []string `json:"links"`
}

func researchReport(docs []researchDocument) (string, bool) {
	byID := map[string]researchDocument{}
	for _, d := range docs {
		byID[d.ID] = d
	}
	expected := map[string]string{
		"D1": "re-executes a fixed program and reuses completed calls",
		"D2": "speculation without primary citation",
		"D3": "renames Anchor Replay to Durable Replay; semantics unchanged",
		"D4": "adds unknown-side-effect fencing",
		"D5": "caches declarative workflow steps; does not replay source code",
	}
	for id, claim := range expected {
		d, ok := byID[id]
		if !ok || d.Claim != claim || d.Date == "" || d.URL == "" || (id != "D2" && !strings.HasPrefix(d.Type, "official_")) {
			return "", false
		}
	}
	ref := func(id string) string { d := byID[id]; return fmt.Sprintf("[%s: %s](%s)", id, d.Title, d.URL) }
	text := "# Anchor Replay 研究报告\n\n范围：固定的本地合成语料 D1–D5，非公开互联网穷尽调查。目标：最早可核实的一手提出日期及后来定义变化。\n\n## 事实\n\n"
	text += fmt.Sprintf("- Atlas 最早可作为证据的一手来源日期为 %s：%s。%s\n", byID["D1"].Date, byID["D1"].Claim, ref("D1"))
	text += fmt.Sprintf("- %s：Anchor Replay 改名为 Durable Replay，语义不变：%s。%s\n", byID["D3"].Date, byID["D3"].Claim, ref("D3"))
	text += fmt.Sprintf("- %s 增加 unknown-side-effect fencing，属于扩展，不能倒推初版已有：%s。%s\n", byID["D4"].Date, byID["D4"].Claim, ref("D4"))
	text += fmt.Sprintf("- 排除 Aster Step Cache（%s）：%s；缓存声明式步骤，不重放源代码，与 Atlas 不等价。%s\n", byID["D5"].Date, byID["D5"].Claim, ref("D5"))
	text += fmt.Sprintf("\n## 推断\n\n在当前语料中，Atlas 是符合固定程序重执行且复用完成调用的项目；改名与安全扩展应分开记录。依据 %s、%s、%s。\n", ref("D1"), ref("D3"), ref("D4"))
	text += fmt.Sprintf("\n## 未发现证据\n\n未发现公开证据支持早于 %s 的一手提出，或初版已有 fencing。D2（%s，二手猜测）只作发现线索，不作为日期结论的依据；未验证语料外的全球最早性。\n", byID["D1"].Date, byID["D2"].Date)
	text += "\n## 后续核查\n\n另一位审阅者可按上述直接链接复核原文与发布日期；若获得更早一手材料，应重新评估时间线。没有创建预约、发送邮件或发布外部内容。\n"
	return text, true
}
