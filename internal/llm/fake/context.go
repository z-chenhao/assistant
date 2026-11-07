package fake

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/z-chenhao/assistant/internal/agent"
	contextbuilder "github.com/z-chenhao/assistant/internal/context"
	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/task"
	"github.com/z-chenhao/assistant/internal/tool"
)

// Transport fixture for the tool-free summary phase. It preserves scripted
// receipt ordinals so protocol tests can continue after real context selection.
// This deliberately makes no claim about semantic summarization quality.
type fixtureContextSummary struct {
	Fixture       bool                `json:"fixture_context_summary"`
	Completed     []string            `json:"completed_call_ids"`
	Batch         fixtureBatchSummary `json:"batch"`
	TaskUpdate    bool                `json:"task_update,omitempty"`
	UnknownEffect bool                `json:"unknown_effect,omitempty"`
	InputRef      string              `json:"input_ref,omitempty"`
}

type fixtureBatchSummary struct {
	Next       int  `json:"next"`
	Succeeded  int  `json:"succeeded"`
	Failed     int  `json:"failed"`
	NeedsInput bool `json:"needs_input"`
}

func contextReply(request llm.GenerationRequest) llm.GenerationResult {
	text := request.Messages[len(request.Messages)-1].Text
	_, evidence, ok := strings.Cut(text, contextbuilder.SummaryEvidenceMarker)
	if !ok {
		return llm.GenerationResult{Text: "Invalid summary fixture evidence."}
	}
	var sources []llm.Message
	if json.NewDecoder(strings.NewReader(evidence)).Decode(&sources) != nil {
		return llm.GenerationResult{Text: "Invalid summary fixture evidence."}
	}
	summary := fixtureContextSummary{Fixture: true}
	seen := map[string]bool{}
	for _, m := range sources {
		old := contextSummary(m)
		if old.InputRef != "" {
			summary.InputRef = old.InputRef
		}
		summary.TaskUpdate = summary.TaskUpdate || old.TaskUpdate || m.Origin == "runtime" && m.Source != nil && m.Source.Kind == "task_update"
		summary.UnknownEffect = summary.UnknownEffect || old.UnknownEffect || strings.Contains(m.Text, "UNKNOWN_SIDE_EFFECT")
		if old.Fixture && old.Batch.Next > summary.Batch.Next {
			summary.Batch = old.Batch
		}
	}
	if _, raw, ok := strings.Cut(text, "Protected task inputs and Host facts (also retained verbatim in the live context):\n"); ok {
		var anchors []llm.Message
		if json.NewDecoder(strings.NewReader(raw)).Decode(&anchors) == nil {
			for _, m := range anchors {
				if m.Role == "user" && m.Origin == "" && m.Source != nil {
					for _, source := range sources {
						for _, call := range source.ToolCalls {
							var control task.ManageRequest
							if call.ID != "effect" || call.Name != task.ManageName || json.Unmarshal([]byte(call.Arguments), &control) != nil {
								continue
							}
							plan := control.Task
							if control.Operation == "append" {
								plan = control.Instruction
							}
							if strings.HasPrefix(plan, "Effect fixture:\n") && fixturePlan(strings.TrimPrefix(plan, "Effect fixture:\n")) == fixturePlan(strings.TrimPrefix(m.Text, "Delegate effect: ")) {
								summary.InputRef = tool.ObjectID(m.Source.Ref)
							}
						}
					}
				}
			}
		}
	}
	previous := summary.Batch.Next
	for _, m := range sources {
		ids := contextReceipts(m)
		if m.Role == "tool" {
			if row, ok := fixtureRow(m.ToolCallID); ok {
				if row > previous {
					summary.Batch.Next = max(summary.Batch.Next, row)
					var result agent.AgentResult
					if json.Unmarshal([]byte(m.Text), &result) == nil && result.Text != "" {
						summary.Batch.Succeeded++
						summary.Batch.NeedsInput = summary.Batch.NeedsInput || strings.HasPrefix(result.Text, "Needs input:")
					} else {
						summary.Batch.Failed++
					}
				}
			} else {
				ids = append(ids, m.ToolCallID)
			}
		}
		for _, id := range ids {
			if id != "" && !seen[id] {
				summary.Completed = append(summary.Completed, id)
				seen[id] = true
			}
		}
	}
	raw, _ := json.Marshal(summary)
	return llm.GenerationResult{Text: string(raw)}
}

func fixturePlan(text string) string {
	var value any
	if json.NewDecoder(strings.NewReader(text)).Decode(&value) != nil {
		return ""
	}
	raw, _ := json.Marshal(value)
	return string(raw)
}

func contextReceipts(m llm.Message) []string {
	return contextSummary(m).Completed
}

// Retrieval and compaction are context, not another scripted user command.
// Keep the Main fixture's completed step when either follows its tool result.
func mainFixtureMessage(messages []llm.Message) llm.Message {
	var last llm.Message
	for _, m := range messages {
		if m.Origin == "runtime" && strings.HasPrefix(m.Text, "Retrieved durable context") {
			continue
		}
		if state := contextSummary(m); state.Fixture {
			for _, id := range state.Completed {
				if id == "echo-a" || id == "echo-b" {
					last = llm.Message{Role: "tool", ToolCallID: id}
				}
			}
			continue
		}
		last = m
	}
	return last
}

func contextSummary(m llm.Message) fixtureContextSummary {
	if m.Source == nil || m.Source.Kind != "working_state" {
		return fixtureContextSummary{}
	}
	_, raw, ok := strings.Cut(m.Text, "\n")
	if !ok {
		return fixtureContextSummary{}
	}
	var envelope struct {
		Content struct {
			Summary string `json:"summary"`
		} `json:"content"`
	}
	if json.Unmarshal([]byte(raw), &envelope) != nil {
		return fixtureContextSummary{}
	}
	var summary fixtureContextSummary
	if json.Unmarshal([]byte(envelope.Content.Summary), &summary) != nil || !summary.Fixture {
		return fixtureContextSummary{}
	}
	return summary
}

func fixtureRow(id string) (int, bool) {
	text, ok := strings.CutPrefix(id, "row-")
	i, e := strconv.Atoi(text)
	return i, ok && e == nil && i > 0
}

func explicitTaskRefs(messages []llm.Message) []string {
	refs := []string{}
	for _, m := range messages {
		if m.Role != "user" || m.Origin == "runtime" {
			continue
		}
		if ref, ok := strings.CutPrefix(m.Text, "Explicit artifact reference: "); ok {
			refs = append(refs, ref)
		}
		for _, label := range []string{"\nExplicit artifact references: ", "\nExplicit artifact IDs: "} {
			if _, raw, ok := strings.Cut(m.Text, label); ok {
				var listed []string
				if json.Unmarshal([]byte(raw), &listed) == nil {
					refs = append(refs, listed...)
				}
			}
		}
	}
	return refs
}
