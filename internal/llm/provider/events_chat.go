package provider

func (s *streamState) consumeChat(api API, v map[string]any) error {
	if s.message.ResponseID == "" {
		s.message.ResponseID = str(v["id"])
	}
	if err := s.usage(record(v["usage"]), api); err != nil {
		return err
	}
	choices := list(v["choices"])
	if len(choices) == 0 {
		return nil
	}
	choice := record(choices[0])
	d := record(choice["delta"])
	if text, ok := d["content"].(string); ok {
		s.text(text, false)
	} else {
		for _, part := range list(d["content"]) {
			p := record(part)
			if p["type"] == "text" {
				s.text(str(p["text"]), false)
			} else if p["type"] == "thinking" {
				s.text(str(p["thinking"]), true)
				for _, chunk := range list(p["thinking"]) {
					s.text(str(record(chunk)["text"]), true)
				}
			}
		}
	}
	s.text(str(d["reasoning_content"]), true)
	s.text(str(d["reasoning"]), true)
	for _, entry := range list(d["tool_calls"]) {
		call := record(entry)
		key := int(integer(call["index"]))
		f := record(call["function"])
		s.tool(key, str(call["id"]), str(f["name"]), "")
		s.args(key, str(f["arguments"]))
	}
	if reason := str(choice["finish_reason"]); reason != "" {
		s.terminal = true
		s.message.StopReason = stopReason(reason)
	}

	return nil
}
