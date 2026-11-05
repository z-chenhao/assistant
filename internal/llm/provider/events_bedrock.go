package provider

import (
	"strings"
)

func (s *streamState) consumeBedrock(api API, v map[string]any) error {
	kind := str(v["type"])
	key := int(integer(v["contentBlockIndex"]))
	switch kind {
	case "contentBlockStart":
		call := record(record(v["start"])["toolUse"])
		if len(call) > 0 {
			s.tool(key, str(call["toolUseId"]), str(call["name"]), "")
		}
	case "contentBlockDelta":
		d := record(v["delta"])
		s.text(str(d["text"]), false)
		if use := record(d["toolUse"]); len(use) > 0 {
			s.args(key, str(use["input"]))
		}
		r := record(d["reasoningContent"])
		s.text(str(r["text"]), true)
		if signature := str(r["signature"]); signature != "" && len(s.message.Content) > 0 {
			s.message.Content[len(s.message.Content)-1].ThinkingSignature += signature
		}
	case "contentBlockStop":
		return s.endTool(key)
	case "messageStop":
		s.terminal = true
		s.message.StopReason = stopReason(str(v["stopReason"]))
	case "metadata":
		return s.usage(record(v["usage"]), api)
	default:
		if strings.Contains(strings.ToLower(kind), "exception") {
			return ErrInvalidResponse
		}
	}

	return nil
}
