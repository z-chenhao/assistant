package fake

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
)

// localReply is a deterministic transport script, enabled only by explicit
// -test-bridge local. It is not evidence of a model's local-life judgment.
func localReply(r llm.GenerationRequest) llm.GenerationResult {
	user, location, at := "", "杭州", ""
	for _, m := range r.Messages {
		if m.Role == "user" && m.Origin == "" {
			user = m.Text
		}
	}
	if strings.Contains(user, "上海") {
		location = "上海"
	}
	if strings.Contains(user, "不可退") {
		return llm.GenerationResult{Text: "涉及显著花费和不可退定金，需转入 GC-13 预订核对：确认门店、时间、人数、金额及退款条款，再按冻结动作申请审批；尚未预订或付款。"}
	}
	if match := regexp.MustCompile(`当前时间：([^。]+)。`).FindStringSubmatch(user); len(match) == 2 {
		at = match[1]
	}
	call := func(refresh bool) llm.GenerationResult {
		raw, _ := json.Marshal(map[string]any{"location": location, "at": at, "refresh": refresh})
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "weather", Name: "mcp__local__weather", Arguments: string(raw)}}}
	}
	last := r.Messages[len(r.Messages)-1]
	if last.Role != "tool" {
		return call(false)
	}
	var data struct {
		Location  string `json:"location"`
		SampledAt string `json:"sampled_at"`
		Available bool   `json:"available"`
		Weather   struct {
			Now int `json:"now_c"`
		} `json:"weather"`
	}
	var prior llm.ToolCall
	for _, m := range r.Messages {
		if len(m.ToolCalls) > 0 {
			prior = m.ToolCalls[len(m.ToolCalls)-1]
		}
	}
	var args struct{ Refresh bool }
	_ = json.Unmarshal([]byte(prior.Arguments), &args)
	if json.Unmarshal([]byte(last.Text), &data) != nil {
		return llm.GenerationResult{Text: "天气来源无法读取，不能确认当前天气。"}
	}
	if !localFresh(location, at, data.Location, data.SampledAt) || !data.Available {
		if !args.Refresh {
			return call(true)
		}
		return llm.GenerationResult{Text: location + "当前天气刷新失败，旧缓存不用于当前建议；暂不能确认穿衣条件，请稍后重查。"}
	}
	clothes := "短袖、薄长裤，随身带薄卫衣"
	if data.Weather.Now < 18 {
		clothes = "长袖、防风外层，带伞"
	}
	return llm.GenerationResult{Text: fmt.Sprintf("%s（采样 %s，%d°C）：%s。开车减少户外暴露；18:30–19:10 散步时留意体感，21:00 下班气温下降时加衣。", data.Location, data.SampledAt, data.Weather.Now, clothes)}
}

func localFresh(location, at, sampledLocation, sampledAt string) bool {
	now, e1 := time.Parse(time.RFC3339, at)
	sample, e2 := time.Parse(time.RFC3339, sampledAt)
	return e1 == nil && e2 == nil && location == sampledLocation && !sample.After(now) && now.Sub(sample) <= 2*time.Hour
}
