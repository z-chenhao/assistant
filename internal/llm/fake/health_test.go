package fake

import (
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/llm"
)

func TestGoldenHealthSafetyAndRecallSuffix(t *testing.T) {
	for _, input := range []string{"同时胸痛、喘不上气", "同时有胸痛，而且喘不上气", "胸痛，呼吸困难"} {
		messages := []llm.Message{
			{Role: "user", Text: "合成背景：轻度脂肪肝"},
			{Role: "user", Text: "我睡醒了，但是好困。"},
			{Role: "user", Text: input},
			{Role: "user", Origin: "runtime", Text: "Retrieved durable context: ordinary morning"},
		}
		reply := goldenReply(messages)
		if !strings.Contains(reply, "立即紧急评估") || strings.Contains(reply, "咖啡") || strings.Contains(reply, "跑步") {
			t.Fatal(input, reply)
		}
	}
	for _, input := range []string{
		"我昨晚一点半才睡，今天没有头晕、胸痛或者呼吸困难。",
		"昨夜01:30才入睡；今天不头晕、不胸痛，呼吸正常。",
		"昨晚1:30才睡，no dizziness/chest pain/breathing trouble today。",
	} {
		reply := goldenReply([]llm.Message{
			{Role: "user", Text: "合成背景：轻度脂肪肝"},
			{Role: "user", Text: "我睡醒了，但是好困。"},
			{Role: "user", Text: input},
			{Role: "user", Origin: "runtime", Text: "Retrieved durable context: 胸痛，呼吸困难"},
		})
		if !strings.Contains(reply, "睡眠不足") || strings.Contains(reply, "紧急评估") {
			t.Fatal(input, reply)
		}
	}
}
