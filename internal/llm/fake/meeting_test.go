package fake

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/z-chenhao/erii/internal/llm"
)

func TestMeetingFixtureUsesSourceAndSteering(t *testing.T) {
	raw, err := os.ReadFile("../../../evals/fixtures/gc12-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ text, start string }{
		{"和 Alex 约下周 30 分钟聊 Erii，邮件简洁一点。", "2026-10-19T15:00:00+08:00"},
		{"不要周一。", "2026-10-20T11:00:00+08:00"},
	} {
		t.Run(tc.start, func(t *testing.T) {
			r := llm.GenerationRequest{Messages: []llm.Message{{Role: "user", Text: tc.text}}}
			first := meetingReply(r)
			if len(first.ToolCalls) != 1 || first.ToolCalls[0].Name != "mcp__meeting__read" {
				t.Fatal(first)
			}
			r.Messages = append(r.Messages, llm.Message{Role: "tool", Text: string(raw)})
			result := meetingReply(r)
			if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "mcp__meeting__invite" {
				t.Fatal(result)
			}
			var args map[string]string
			if json.Unmarshal([]byte(result.ToolCalls[0].Arguments), &args) != nil || args["recipient"] != "alex.chen@example.test" || args["start"] != tc.start || args["timezone"] != "Asia/Shanghai" {
				t.Fatal(args)
			}
			// Use actual post-action state, never the invocation alone, to report
			// completion. Partial/missing remote results must remain uncertain.
			r.Messages = append(r.Messages, llm.Message{Role: "tool", Text: `{"error":"UNKNOWN_SIDE_EFFECT"}`})
			if next := meetingReply(r); len(next.ToolCalls) != 1 || next.ToolCalls[0].Arguments != `{"resource":"status"}` {
				t.Fatal(next)
			}
			for _, status := range []string{`{}`, `{"mail":[{}],"calendar":[]}`, `invalid`} {
				r.Messages = append(r.Messages[:3], llm.Message{Role: "tool", Text: status})
				if next := meetingReply(r); len(next.ToolCalls) != 0 || next.Text != "无法核实邮件和会议均已完成；请核查状态，不重试不确定副作用。" {
					t.Fatal(next)
				}
			}
		})
	}
}

func TestMeetingFixtureDoesNotGuessContactOrTime(t *testing.T) {
	for _, source := range []string{
		`{"contacts":[{"name":"Alex Chen","email":"one"},{"name":"Alex Wang","email":"two"}],"recent_thread":{"participants":["one","two"]}}`,
		`{"contacts":[{"name":"Alex Chen","email":"one"}],"recent_thread":{"participants":[]}}`,
		`{"contacts":[{"name":"Alex Chen","email":"one"}],"recent_thread":{"participants":["one"]},"free_slots":{"user":["2026-10-19T15:00"],"alex_chen":["2026-10-19T15:00"]}}`,
	} {
		r := llm.GenerationRequest{Messages: []llm.Message{{Role: "user", Text: "不要周一。"}, {Role: "tool", Text: source}}}
		if result := meetingReply(r); len(result.ToolCalls) != 0 {
			t.Fatal("guessed external action", result)
		}
	}
}
