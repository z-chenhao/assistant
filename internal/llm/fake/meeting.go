package fake

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
)

// Explicit GC-12 deterministic model fixture. It reads source through the real
// MCP Tool and verifies remote state after dispatch; it cannot write either DB.
// This proves runner mechanics, never semantic model quality or L2 acceptance.
func meetingReply(r llm.GenerationRequest) llm.GenerationResult {
	start := 0
	user := ""
	for i, m := range r.Messages {
		if m.Role == "user" && m.Origin == "" {
			start, user = i, m.Text
		}
	}
	var results []string
	for _, m := range r.Messages[start+1:] {
		if m.Role == "tool" {
			results = append(results, m.Text)
		}
	}
	call := func(name string, args any) llm.GenerationResult {
		raw, _ := json.Marshal(args)
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "meeting-fixture", Name: "mcp__meeting__" + name, Arguments: string(raw)}}}
	}
	if len(results) == 0 {
		return call("read", map[string]string{"resource": "source"})
	}
	// The Loop projects the common Tool Result's Text into the Tool message.
	unwrap := func(raw string, target any) bool {
		return json.Unmarshal([]byte(raw), target) == nil
	}
	if len(results) == 1 {
		var source struct {
			Contacts     []struct{ Name, Email, Relation string }
			RecentThread struct {
				Participants []string
				Subject      string
			} `json:"recent_thread"`
			FreeSlots struct {
				User     []string
				AlexChen []string `json:"alex_chen"`
			} `json:"free_slots"`
		}
		if !unwrap(results[0], &source) {
			return llm.GenerationResult{Text: "无法读取联系人与日历证据，未发送或建会。"}
		}
		var recipients []string
		for _, contact := range source.Contacts {
			if !strings.HasPrefix(contact.Name, "Alex ") {
				continue
			}
			for _, participant := range source.RecentThread.Participants {
				if contact.Email == participant {
					recipients = append(recipients, contact.Email)
					break
				}
			}
		}
		if len(recipients) != 1 {
			return llm.GenerationResult{Text: "Alex 的身份仍有歧义，请确认收件人；未发送或建会。"}
		}
		var chosen time.Time
		for _, slot := range source.FreeSlots.User {
			at, err := time.Parse("2006-01-02T15:04", slot)
			if err != nil || (strings.Contains(user, "不要周一") || strings.Contains(user, "not Monday")) && at.Weekday() == time.Monday {
				continue
			}
			for _, other := range source.FreeSlots.AlexChen {
				if slot == other && (chosen.IsZero() || at.Before(chosen)) {
					chosen = at
				}
			}
		}
		if chosen.IsZero() {
			return llm.GenerationResult{Text: "没有已证实的共同空档，请提供时间；未发送或建会。"}
		}
		return call("invite", map[string]string{
			"action": "send_mail_and_create_calendar", "recipient": recipients[0],
			"subject": source.RecentThread.Subject,
			"body":    "Alex，你好！想约你聊 Erii，" + chosen.Format("2006-01-02 15:04") + " 开始，30 分钟。谢谢！",
			"start":   chosen.Format("2006-01-02T15:04:05") + "+08:00",
			"end":     chosen.Add(30*time.Minute).Format("2006-01-02T15:04:05") + "+08:00", "timezone": "Asia/Shanghai",
		})
	}
	if len(results) == 2 {
		return call("read", map[string]string{"resource": "status"})
	}
	var status struct {
		Mail     []json.RawMessage
		Calendar []json.RawMessage
	}
	if !unwrap(results[len(results)-1], &status) || len(status.Mail) != 1 || len(status.Calendar) != 1 {
		return llm.GenerationResult{Text: "无法核实邮件和会议均已完成；请核查状态，不重试不确定副作用。"}
	}
	return llm.GenerationResult{Text: fmt.Sprintf("已核对：邮件 %d 封、日历会议 %d 个。邮件状态：%s；会议状态：%s", len(status.Mail), len(status.Calendar), status.Mail[0], status.Calendar[0])}
}
