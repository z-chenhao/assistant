package fake

import (
	"encoding/json"
	"github.com/z-chenhao/erii/internal/llm"
	"strings"
	"testing"
)

func TestLocalFixtureFreshnessBoundaries(t *testing.T) {
	at := "2026-10-12T08:10:00+08:00"
	for _, c := range []struct {
		location, sample string
		fresh            bool
	}{
		{"杭州", "2026-10-12T06:10:00+08:00", true},
		{"杭州", "2026-10-11T22:10:00Z", true},
		{"杭州", "2026-10-12T06:09:59+08:00", false},
		{"杭州", "2026-10-12T08:10:01+08:00", false},
		{"上海", "2026-10-12T08:09:00+08:00", false},
		{"杭州", "invalid", false},
	} {
		if got := localFresh("杭州", at, c.location, c.sample); got != c.fresh {
			t.Fatalf("%+v got %v", c, got)
		}
	}
}

func TestLocalFixtureRefreshFailureStopsAndCostDoesNotAct(t *testing.T) {
	r := llm.GenerationRequest{Messages: []llm.Message{{Role: "user", Text: "当前时间：2026-10-12T08:10:00+08:00。今天穿什么？"}}}
	first := localReply(r)
	if len(first.ToolCalls) != 1 || first.ToolCalls[0].Name != "mcp__local__weather" {
		t.Fatal(first)
	}
	r.Messages = append(r.Messages, llm.Message{Role: "assistant", ToolCalls: first.ToolCalls}, llm.Message{Role: "tool", Text: `{"location":"杭州","sampled_at":"2026-10-12T06:09:59+08:00","available":true,"weather":{"now_c":19}}`})
	refresh := localReply(r)
	var args map[string]any
	if len(refresh.ToolCalls) != 1 {
		t.Fatal(refresh)
	}
	if err := json.Unmarshal([]byte(refresh.ToolCalls[0].Arguments), &args); err != nil || args["refresh"] != true {
		t.Fatal(refresh)
	}
	r.Messages = append(r.Messages, llm.Message{Role: "assistant", ToolCalls: refresh.ToolCalls}, llm.Message{Role: "tool", Text: `{"location":"杭州","sampled_at":"2026-10-12T08:09:00+08:00","available":false,"weather":null}`})
	failed := localReply(r)
	if len(failed.ToolCalls) != 0 || !strings.Contains(failed.Text, "刷新失败") {
		t.Fatal(failed)
	}
	r.Messages = append(r.Messages, llm.Message{Role: "user", Text: "不可退的200元定金，请安排"})
	cost := localReply(r)
	if len(cost.ToolCalls) != 0 || !strings.Contains(cost.Text, "GC-13") || !strings.Contains(cost.Text, "尚未预订或付款") {
		t.Fatal(cost)
	}
}
