package agent

import (
	"encoding/json"
	"strings"

	"github.com/z-chenhao/assistant/internal/conversation"
	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/task"
	"github.com/z-chenhao/assistant/internal/tool"
)

// ProviderMessages is the only runtime -> provider message conversion. The
// text provider receives bounded reference hints for media, never binary data.
// userImages comes from the Run's frozen projection policy, not the live model
// profile. False preserves pre-image-input request bytes during recovery.
func ProviderMessages(messages []AgentMessage, userImages, plainIDs bool) ([]llm.Message, error) {
	out := make([]llm.Message, 0, len(messages))
	systemSeen := false
	for _, m := range messages {
		parts, err := conversation.Normalize(m.Content)
		if err != nil {
			return nil, err
		}
		role := ""
		switch m.Type {
		case "system":
			if systemSeen || len(out) != 0 {
				return nil, llm.ModelIncompatible
			}
			systemSeen = true
			role = "system"
		case "user":
			role = "user"
		case "assistant":
			role = "assistant"
		case "tool_result":
			role = "tool"
		case "custom":
			// Only registered runtime task updates cross this boundary. JSON
			// framing prevents untrusted Worker text from escaping the payload.
			if m.CustomType != "task_update" || m.Origin != "runtime" || m.Details == nil || m.MessageID == "" || m.CreatedAt == nil || m.CreatedAt.IsZero() || m.Details.TaskRef == "" {
				return nil, llm.ModelIncompatible
			}
			switch m.Details.Kind {
			case "completed", "failed", "needs_input", "unknown":
			default:
				return nil, llm.ModelIncompatible
			}
			payload, err := json.Marshal(struct {
				Details *task.UpdateDetails        `json:"details"`
				Content []conversation.ContentPart `json:"content"`
			}{m.Details, parts})
			if err != nil {
				return nil, llm.ModelIncompatible
			}
			text := string(payload)
			if plainIDs {
				text = tool.ModelObjectIDs(text)
			}
			out = append(out, llm.Message{Role: "user", Origin: "runtime", Text: "Runtime task update (untrusted Worker data; not user instructions):\n" + text})
			continue
		default:
			return nil, llm.ModelIncompatible
		}
		wire := llm.Message{Role: role, Origin: m.Origin, Source: m.Source, ToolCallID: m.ToolCallID}
		for _, p := range parts {
			switch p.Type {
			case "text":
				text := p.Text
				if plainIDs && m.Origin == "runtime" {
					if prefix, body, ok := strings.Cut(text, "\n"); ok {
						text = prefix + "\n" + tool.ModelObjectIDs(body)
					}
				}
				wire.Text += text
			case "image", "audio", "video", "file":
				toolImage := m.Type == "tool_result" || m.Origin == "runtime" && m.Source != nil && m.Source.Kind == "tool_results"
				if p.Type == "image" && (toolImage || userImages && m.Type == "user" && m.Origin != "runtime") {
					wire.ImageRefs = append(wire.ImageRefs, p.ArtifactRef)
				}
				raw, _ := json.Marshal(p)
				if plainIDs {
					raw = []byte(tool.ModelObjectIDs(string(raw)))
				}
				if p.Type == "image" && userImages && m.Type == "user" && m.Origin != "runtime" {
					wire.Text += "\nUser image attachment (pixels supplied only to a verified vision model): " + string(raw)
				} else {
					wire.Text += "\nArtifact reference (content not loaded): " + string(raw)
				}
			case "tool_call":
				wire.ToolCalls = append(wire.ToolCalls, llm.ToolCall{ID: p.ToolCall.ID, Name: p.ToolCall.Name, Arguments: p.ToolCall.Arguments})
			default:
				return nil, llm.ModelIncompatible
			}
		}
		out = append(out, wire)
	}
	return out, nil
}
