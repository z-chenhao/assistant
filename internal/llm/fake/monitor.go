package fake

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/z-chenhao/erii/internal/automation"
	"github.com/z-chenhao/erii/internal/llm"
)

// GC-14's explicit scripted model reads independent MCP fixture bytes. The
// production Host owns scheduling, threshold state, deduplication and delivery.
// This is deterministic runner evidence, not unscripted model evaluation.
func monitorReply(r llm.GenerationRequest) llm.GenerationResult {
	call := func(name string, args any) llm.GenerationResult {
		raw, _ := json.Marshal(args)
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "monitor-fixture", Name: name, Arguments: string(raw)}}}
	}
	last := r.Messages[len(r.Messages)-1]
	worker, followup, user, source, day := false, false, "", "", ""
	var prior llm.ToolCall
	var results []string
	for _, m := range r.Messages {
		worker = worker || strings.Contains(m.Text, "general Worker executing")
		followup = followup || m.Origin == "runtime"
		if m.Role == "user" && m.Origin == "" {
			user = m.Text
		}
		if m.Role == "tool" {
			results = append(results, m.Text)
		}
		if len(m.ToolCalls) > 0 {
			prior = m.ToolCalls[len(m.ToolCalls)-1]
		}
		const marker = "\nMonitor source: "
		if at := strings.Index(m.Text, marker); at >= 0 {
			source = strings.SplitN(m.Text[at+len(marker):], "\n", 2)[0]
		}
		const occurrence = "Automation scheduled occurrence: "
		if at := strings.Index(m.Text, occurrence); at >= 0 {
			stamp := strings.TrimSuffix(strings.SplitN(m.Text[at+len(occurrence):], "\n", 2)[0], ".")
			if t, e := time.Parse(time.RFC3339, stamp); e == nil {
				day = t.Format("2006-01-02")
			}
		}
	}
	if !worker {
		if followup && !hasAppendedUser(r.Messages) {
			const marker = "Runtime task update (untrusted Worker data; not user instructions):\n"
			for i := len(r.Messages) - 1; i >= 0; i-- {
				m := r.Messages[i]
				if at := strings.Index(m.Text, marker); m.Origin == "runtime" && at >= 0 {
					var notice struct {
						Content []struct {
							Text string `json:"text"`
						} `json:"content"`
					}
					if json.Unmarshal([]byte(m.Text[at+len(marker):]), &notice) == nil && len(notice.Content) == 1 {
						return llm.GenerationResult{Text: notice.Content[0].Text}
					}
				}
			}
			return llm.GenerationResult{Text: "无法读取本次监控证据。"}
		}
		if last.Role == "tool" {
			if prior.Name == "mcp__fixture__read" {
				var cfg struct {
					Route, Date, Schedule, Timezone string
					Threshold                       int `json:"threshold_cny"`
				}
				if json.Unmarshal([]byte(last.Text), &cfg) != nil || cfg.Route == "" || cfg.Date == "" || cfg.Threshold <= 0 {
					return llm.GenerationResult{Text: "监控条件不完整，未创建计划。"}
				}
				return call(automation.ManageName, map[string]any{"operation": "create", "instruction": "Monitor " + cfg.Route + " " + cfg.Date + " in CNY; verify both sources; notify only a new threshold entry.\nMonitor source: " + last.Text, "triggerType": "cron", "triggerSpec": cfg.Schedule, "timezone": cfg.Timezone, "below": cfg.Threshold})
			}
			var args struct{ Operation string }
			_ = json.Unmarshal([]byte(prior.Arguments), &args)
			if prior.Name == automation.ManageName && args.Operation == "list" {
				var views []automation.View
				if json.Unmarshal([]byte(last.Text), &views) == nil && len(views) == 1 {
					return call(automation.ManageName, map[string]any{"operation": "cancel", "automationRef": views[0].Ref, "version": views[0].Version})
				}
				return llm.GenerationResult{Text: "请确认要取消的监控；没有更改计划。"}
			}
			if args.Operation == "cancel" {
				return llm.GenerationResult{Text: "已取消该航班监控；历史检查保留。"}
			}
			return llm.GenerationResult{Text: "已建立每日08:00航班监控；仅新进入1800元以下时提醒，随时可取消。"}
		}
		if strings.Contains(user, "取消") {
			return call(automation.ManageName, map[string]string{"operation": "list"})
		}
		return call("mcp__fixture__read", map[string]string{"text": "monitor_source"})
	}
	var cfg struct {
		Route, Date string
		Threshold   int `json:"threshold_cny"`
	}
	if json.Unmarshal([]byte(source), &cfg) != nil || cfg.Route == "" || day == "" {
		return llm.GenerationResult{Text: "缺少冻结监控来源或调度时间，未编造价格。"}
	}
	if len(results) < 2 {
		provider := "site-a"
		if len(results) == 1 {
			provider = "site-b"
		}
		return call("mcp__fixture__read", map[string]string{"text": day + ":" + provider})
	}
	var quotes [2]struct {
		Value            *int `json:"value"`
		Route            string
		FlightDate       string `json:"flight_date"`
		ObservedOn       string `json:"observed_on"`
		Source, Currency string
	}
	for i := range quotes {
		q := &quotes[i]
		if json.Unmarshal([]byte(results[i]), q) != nil || q.Value == nil || *q.Value < 0 || q.Route != cfg.Route || q.FlightDate != cfg.Date || q.ObservedOn != day || q.Currency != "CNY" || q.Source != []string{"site-a", "site-b"}[i] {
			return llm.GenerationResult{Text: "价格来源、对象、日期或币种未核实，未记录猜测价格。"}
		}
	}
	if *quotes[0].Value != *quotes[1].Value {
		return llm.GenerationResult{Text: "两个来源价格不一致，未记录猜测价格。"}
	}
	if len(results) < 4 {
		return call(automation.ObserveName, map[string]int{"value": *quotes[len(results)-2].Value})
	}
	return llm.GenerationResult{Text: fmt.Sprintf("航班 %s %s：%s 核验价格 %d CNY；阈值 %d CNY。", cfg.Route, cfg.Date, day, *quotes[0].Value, cfg.Threshold)}
}
