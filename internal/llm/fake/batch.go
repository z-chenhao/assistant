package fake

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/z-chenhao/collie/internal/agent"
	"github.com/z-chenhao/collie/internal/llm"
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
	state := fixtureBatchSummary{}
	for _, m := range req.Messages {
		if summary := contextSummary(m); summary.Fixture && summary.Batch.Next > state.Next {
			state = summary.Batch
		}
		for _, c := range m.ToolCalls {
			if c.Name == agent.RunToolName {
				if row, ok := fixtureRow(c.ID); ok {
					count = max(count, row)
				}
			}
		}
	}
	count = max(count, state.Next)
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
	success, failed := state.Succeeded, state.Failed
	blockers := map[string]bool{}
	if state.NeedsInput {
		blockers["Needs input: confirm region"] = true
	}
	for _, m := range req.Messages {
		if m.Role == "tool" {
			if row, ok := fixtureRow(m.ToolCallID); ok && row <= state.Next {
				continue
			}
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
