package provider

func (s *streamState) consumeAnthropic(api API, v map[string]any) error {
	kind := str(v["type"])
	key := int(integer(v["index"]))
	switch kind {
	case "message_start":
		if err := s.usage(record(record(v["message"])["usage"]), api); err != nil {
			return err
		}
	case "content_block_start":
		b := record(v["content_block"])
		switch b["type"] {
		case "tool_use":
			p := s.tool(key, str(b["id"]), str(b["name"]), "")
			if a, ok := b["input"].(map[string]any); ok {
				s.message.Content[p.index].Arguments = a
			}
		case "text":
			s.text(str(b["text"]), false)
		case "thinking":
			s.text(str(b["thinking"]), true)
		case "redacted_thinking":
			s.message.Content = append(s.message.Content, Content{Type: "thinking", Redacted: true, ThinkingSignature: str(b["data"])})
		}
	case "content_block_delta":
		d := record(v["delta"])
		switch d["type"] {
		case "text_delta":
			s.text(str(d["text"]), false)
		case "thinking_delta":
			s.text(str(d["thinking"]), true)
		case "input_json_delta":
			s.args(key, str(d["partial_json"]))
		case "signature_delta":
			for i := len(s.message.Content) - 1; i >= 0; i-- {
				if s.message.Content[i].Type == "thinking" {
					s.message.Content[i].ThinkingSignature += str(d["signature"])
					break
				}
			}
		}
	case "content_block_stop":
		return s.endTool(key)
	case "message_delta":
		if reason := str(record(v["delta"])["stop_reason"]); reason != "" {
			s.message.StopReason = stopReason(reason)
		}
		if err := s.usage(record(v["usage"]), api); err != nil {
			return err
		}
	case "message_stop":
		s.terminal = true
	case "error":
		return ErrInvalidResponse
	}

	return nil
}
