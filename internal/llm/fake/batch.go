package fake

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/llm"
)

// batchReply is deterministic Direct Tool evidence. Four 25-call exchanges keep
// the existing 32-call Bridge frame contract, while one Root processes 100/1000.
func batchReply(req llm.GenerationRequest, input string, root bool) (llm.GenerationResult, error) {
	if !root {
		i, err := strconv.Atoi(strings.TrimPrefix(input, "Batch item "))
		if err != nil {
			return llm.GenerationResult{}, llm.InvalidProviderResponse
		}
		if i == 37 || i == 74 {
			return llm.GenerationResult{}, llm.ModelNotFound
		}
		text := fmt.Sprintf("row %03d: classified", i)
		if i == 7 || i == 8 {
			text = "Needs input: confirm region"
		}
		return llm.GenerationResult{Text: text}, nil
	}
	count := 0
	for _, m := range req.Messages {
		for _, c := range m.ToolCalls {
			if c.Name == agent.RunToolName {
				count++
			}
		}
	}
	target := 100
	if strings.Contains(input, "Batch 1000") {
		target = 1000
	}
	if count < target {
		result := llm.GenerationResult{}
		for i := count + 1; i <= count+25; i++ {
			text := fmt.Sprintf("Batch item %d", i)
			if strings.Contains(input, "Batch hold") {
				text = "Hold branch"
			}
			raw, _ := json.Marshal(agent.AgentRunRequest{Task: text})
			result.ToolCalls = append(result.ToolCalls, llm.ToolCall{ID: fmt.Sprintf("row-%03d", i), Name: agent.RunToolName, Arguments: string(raw)})
		}
		return result, nil
	}
	success, failed := 0, 0
	blockers := map[string]bool{}
	for _, m := range req.Messages {
		if m.Role == "tool" {
			var result agent.AgentResult
			if json.Unmarshal([]byte(m.Text), &result) == nil && result.Text != "" {
				success++
				if strings.HasPrefix(result.Text, "Needs input:") {
					blockers[result.Text] = true
				}
			} else {
				failed++
			}
		}
	}
	text := fmt.Sprintf("Batch aggregate: %d results, %d failed.", success, failed)
	if len(blockers) > 0 {
		text += " Needs input: confirm region"
	}
	return llm.GenerationResult{Text: text}, nil
}
