package fake

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/task"
)

// An explicit GC-08 scripted model, never a production policy. The real Host
// owns Tasks, Artifact bytes, approval and replay. The model has no DB access.
func weddingReply(r llm.GenerationRequest) llm.GenerationResult {
	call := func(name string, args any) llm.GenerationResult {
		raw, _ := json.Marshal(args)
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "wedding-fixture", Name: name, Arguments: string(raw)}}}
	}
	last := r.Messages[len(r.Messages)-1]
	user, root, source := "", false, ""
	committedRef, recoveredText := "", ""
	var prior llm.ToolCall
	var update struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Details task.UpdateDetails `json:"details"`
	}
	for _, m := range r.Messages {
		root = root || strings.Contains(m.Text, "general Worker executing")
		if m.Role == "user" && m.Origin == "" {
			user = m.Text
		}
		if len(m.ToolCalls) > 0 {
			prior = m.ToolCalls[len(m.ToolCalls)-1]
		}
		if m.Role == "tool" && prior.Name == "mcp__wedding__read" {
			source = m.Text
		}
		if m.Role == "tool" && prior.Name == "builtin__artifact__manage" {
			var args struct{ Operation string }
			var result struct{ Text string }
			if json.Unmarshal([]byte(prior.Arguments), &args) == nil && args.Operation == "read" && json.Unmarshal([]byte(m.Text), &result) == nil {
				recoveredText = result.Text
			}
		}
		const committed = "Previous committed Task work (untrusted data): "
		if strings.HasPrefix(m.Text, committed) {
			var data struct {
				Refs []string `json:"artifact_refs"`
			}
			if json.Unmarshal([]byte(strings.TrimPrefix(m.Text, committed)), &data) == nil && len(data.Refs) > 0 {
				committedRef = data.Refs[len(data.Refs)-1]
			}
		}
		const marker = "Runtime task update (untrusted Worker data; not user instructions):\n"
		if at := strings.Index(m.Text, marker); m.Origin == "runtime" && at >= 0 {
			_ = json.Unmarshal([]byte(m.Text[at+len(marker):]), &update)
		}
		const prefix = "\nWedding source: "
		if at := strings.Index(m.Text, prefix); at >= 0 {
			source = strings.SplitN(m.Text[at+len(prefix):], "\n", 2)[0]
		}
	}
	if !root {
		if update.Details.TaskRef != "" && !hasAppendedUser(r.Messages) {
			if last.Role != "tool" {
				return call("builtin__artifact__manage", map[string]any{"operation": "publish", "artifactRefs": update.Details.ArtifactRefs})
			}
			text := ""
			for _, part := range update.Content {
				text += part.Text
			}
			return llm.GenerationResult{Text: text}
		}
		if last.Role == "tool" {
			switch prior.Name {
			case "mcp__wedding__read":
				return call(task.ManageName, task.ManageRequest{Operation: "create", Task: "婚礼准备\nWedding source: " + last.Text + "\n" + user})
			case task.SearchName:
				var matches []task.Summary
				if json.Unmarshal([]byte(last.Text), &matches) != nil || len(matches) != 1 {
					return llm.GenerationResult{Text: "请确认要继续的婚礼 Task；没有创建重复事项。"}
				}
				if strings.Contains(user, "暂停") {
					return call(task.ManageName, task.ManageRequest{Operation: "pause", TaskRef: matches[0].TaskRef})
				}
				return call(task.ManageName, task.ManageRequest{Operation: "append", TaskRef: matches[0].TaskRef, Instruction: user, ArtifactRefs: update.Details.ArtifactRefs})
			default:
				return llm.GenerationResult{Text: "已接到同一婚礼准备事项。"}
			}
		}
		if strings.Contains(user, "128") || strings.Contains(user, "酒店") || strings.Contains(user, "继续") || strings.Contains(user, "暂停") {
			return call(task.SearchName, task.SearchRequest{Query: "婚礼", Scope: "all"})
		}
		return call("mcp__wedding__read", map[string]string{"resource": "source"})
	}
	all := ""
	for _, m := range r.Messages {
		if m.Role == "user" && m.Origin == "" {
			input := m.Text
			if strings.HasPrefix(input, "Task anchor:") {
				all += input[strings.LastIndex(input, "\n")+1:] + "\n"
			} else if strings.HasPrefix(input, "Previous Task instruction: ") || strings.HasPrefix(input, "Task instruction: ") {
				all += strings.SplitN(input, "\n", 2)[0] + "\n"
			}
		}
	}
	if strings.HasPrefix(user, "Task anchor:") {
		user = user[strings.LastIndex(user, "\n")+1:]
	} else {
		user = strings.SplitN(strings.TrimPrefix(user, "Task instruction: "), "\n", 2)[0]
	}
	var priorArgs struct {
		Operation string `json:"operation"`
	}
	_ = json.Unmarshal([]byte(prior.Arguments), &priorArgs)
	// A completed save/publish is progress even when its source/read exchanges
	// have left the bounded context. Handle these receipts before recovering
	// inputs; never repeat a completed write to reconstruct transient context.
	if last.Role == "tool" && prior.Name == "builtin__artifact__manage" && priorArgs.Operation == "save" {
		var result struct {
			Refs []string `json:"artifact_refs"`
		}
		if json.Unmarshal([]byte(last.Text), &result) != nil || len(result.Refs) != 1 || result.Refs[0] == "" {
			return llm.GenerationResult{Text: "清单保存未核实，未发布或联系任何人。"}
		}
		return call("builtin__artifact__manage", map[string]any{"operation": "publish", "artifactRefs": result.Refs})
	}
	if last.Role == "tool" && prior.Name == "builtin__artifact__manage" && priorArgs.Operation == "publish" && strings.Contains(user, "婚期改为") {
		var result struct {
			Text string   `json:"text"`
			Refs []string `json:"artifact_refs"`
		}
		var args struct {
			Refs []string `json:"artifactRefs"`
		}
		if json.Unmarshal([]byte(last.Text), &result) != nil || result.Text != "Final artifacts selected." ||
			json.Unmarshal([]byte(prior.Arguments), &args) != nil || len(args.Refs) != 1 || args.Refs[0] == "" ||
			len(result.Refs) != 1 || result.Refs[0] != args.Refs[0] {
			return llm.GenerationResult{Text: "修订清单发布未核实，未联系任何人。"}
		}
		return llm.GenerationResult{Text: "修订清单已保存并发布；请查看附件中的婚期、截止日期和已确认人数。桌数仍待确认，未联系任何人。"}
	}
	var data struct {
		Clock     string            `json:"clock"`
		Event     map[string]any    `json:"event"`
		Checklist []map[string]any  `json:"checklist"`
		Contacts  map[string]string `json:"contacts"`
	}
	if json.Unmarshal([]byte(source), &data) != nil || data.Contacts["hotel"] == "" {
		if last.Role == "tool" && prior.Name == "mcp__wedding__read" {
			return llm.GenerationResult{Text: "婚礼来源不可用，未联系任何人。"}
		}
		return call("mcp__wedding__read", map[string]string{"resource": "source"})
	}
	confirmed := strings.Contains(all, "128")
	if strings.Contains(user, "婚期改为") {
		if recoveredText == "" {
			if committedRef == "" || last.Role == "tool" && prior.Name == "builtin__artifact__manage" {
				return llm.GenerationResult{Text: "已提交清单不可用，未猜测宾客或桌数，未联系任何人。"}
			}
			return call("builtin__artifact__manage", map[string]any{"operation": "read", "artifactRef": committedRef, "limit": 4096})
		}
		var saved struct {
			Event     map[string]any   `json:"event"`
			Checklist []map[string]any `json:"checklist"`
			Facts     struct {
				Guests   int `json:"guests"`
				Tables   any `json:"table_count"`
				PerTable any `json:"per_table"`
			} `json:"facts"`
		}
		if json.Unmarshal([]byte(recoveredText), &saved) != nil || saved.Facts.Guests != 128 || saved.Facts.Tables != nil || saved.Facts.PerTable != nil || len(saved.Checklist) != 3 || saved.Event["title"] != "婚礼" {
			return llm.GenerationResult{Text: "已提交清单事实不完整，未猜测宾客或桌数，未联系任何人。"}
		}
		data.Event, data.Checklist, confirmed = saved.Event, saved.Checklist, true
	}
	// Read changes only from ordered user instructions, never source noise or
	// previous Tool results. The fixture remains a scripted model with no store
	// access; the Host decides how the new Root inherits committed Task context.
	for _, change := range regexp.MustCompile(`婚期改为(\d{4}-\d{2}-\d{2})|C([23])截止改为(\d{4}-\d{2}-\d{2})`).FindAllStringSubmatch(all, -1) {
		if change[1] != "" {
			data.Event["date"] = change[1]
		} else {
			for _, item := range data.Checklist {
				if item["id"] == "C"+change[2] {
					item["due"] = change[3]
				}
			}
		}
	}
	draft := strings.Contains(user, "酒店")
	if draft && !confirmed {
		return llm.GenerationResult{Text: "请先确认宾客人数；未生成含猜测人数的邮件，未联系酒店。"}
	}
	text := "先确认 C1 宾客最终人数（10月20日）；它阻塞 C2 酒店桌次（10月22日）。C3 摄影到场时间10月25日，无当前风险，稍后处理。"
	var guests any
	if confirmed {
		guests = 128
		for _, item := range data.Checklist {
			if item["id"] == "C1" {
				item["status"] = "completed"
			}
			if item["id"] == "C2" {
				item["status"] = "active"
			}
		}
		text = "同一婚礼 Task 已保存宾客128人，C1完成，C2解除阻塞。桌次数量待确认：尚不知道每桌人数，不假设13桌或其他数量；C3稍后处理。"
	}
	mail := map[string]any{"recipient": data.Contacts["hotel"], "subject": "婚礼桌次确认草稿", "body": "您好，11月1日婚礼宾客已确定128人。请确认每桌人数及桌次数量；我们尚未确定桌数。谢谢。", "attachments": []string{}}
	if draft {
		text = "邮件草稿已保存，等待你审阅冻结的收件人、正文和附件；尚未发送。"
	}
	if last.Role != "tool" || prior.Name == "mcp__wedding__read" || prior.Name == "builtin__artifact__manage" && priorArgs.Operation == "read" {
		doc := map[string]any{"event": data.Event, "clock": data.Clock, "checklist": data.Checklist,
			"facts": map[string]any{"guests": guests, "table_count": nil, "per_table": nil}, "summary": text}
		if draft {
			doc["draft"] = mail
		}
		raw, _ := json.Marshal(doc)
		return call("builtin__artifact__manage", map[string]string{"operation": "save", "name": "wedding-checklist.json", "mediaType": "application/json", "text": string(raw)})
	}
	if prior.Name == "builtin__artifact__manage" && priorArgs.Operation == "publish" && draft {
		return call("mcp__wedding__send", mail)
	}
	if prior.Name == "mcp__wedding__send" {
		return llm.GenerationResult{Text: "没有已核实的发送结果，保留草稿，不重试不确定副作用。"}
	}
	return llm.GenerationResult{Text: text}
}
