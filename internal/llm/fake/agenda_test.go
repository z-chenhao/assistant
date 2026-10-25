package fake

import (
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/llm"
)

func TestAgendaFixtureSteeringDoesNotRepeatCompletedWork(t *testing.T) {
	messages := []llm.Message{
		{Role: "user", Text: "合成背景：发布风险说明 E1/T1；E4面试确认；14:00面试"},
		{Role: "user", Text: "我今天先做什么？"},
	}
	initial := goldenReply(messages)
	if !strings.Contains(initial, "E1/T1") {
		t.Fatal(initial)
	}
	messages = append(messages, llm.Message{Role: "assistant", Text: initial},
		llm.Message{Role: "user", Text: "E1/T1已完成，E4已核对；13:00–14:00处理私人事务。请更新安排，面试仍在14:00，不再安排已完成事项。"},
		llm.Message{Role: "tool", Text: "echo"})
	updated := goldenReply(messages)
	for _, completed := range []string{"E1", "T1", "E4", "10:00", "13:30"} {
		if strings.Contains(updated, completed) {
			t.Fatalf("completed or superseded work %q in updated agenda: %s", completed, updated)
		}
	}
	for _, current := range []string{"12:30–13:00", "14:00面试", "13:00–14:00", "T2", "E2", "E3"} {
		if !strings.Contains(updated, current) {
			t.Fatalf("missing current constraint %q: %s", current, updated)
		}
	}
}
