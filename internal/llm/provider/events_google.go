package provider

import (
	"encoding/json"
)

func (s *streamState) consumeGoogle(api API, v map[string]any) error {
	if err := s.usage(record(v["usageMetadata"]), api); err != nil {
		return err
	}
	for _, candidate := range list(v["candidates"]) {
		c := record(candidate)
		for _, part := range list(record(c["content"])["parts"]) {
			p := record(part)
			if text := str(p["text"]); text != "" {
				thinking, _ := p["thought"].(bool)
				s.text(text, thinking)
				if signature := str(p["thoughtSignature"]); signature != "" {
					s.message.Content[len(s.message.Content)-1].ThinkingSignature = signature
				}
			}
			if call, ok := p["functionCall"].(map[string]any); ok {
				key := len(s.tools)
				t := s.tool(key, str(call["id"]), str(call["name"]), "")
				raw, _ := json.Marshal(record(call["args"]))
				t.arguments.Write(raw)
				if err := s.endTool(key); err != nil {
					return err
				}
				s.message.Content[t.index].ThinkingSignature = str(p["thoughtSignature"])
			}
		}
		if reason := str(c["finishReason"]); reason != "" {
			s.terminal = true
			s.message.StopReason = stopReason(reason)
		}
	}

	return nil
}
