package provider

import (
	"encoding/json"
)

func (s *streamState) consumeGateway(api API, v map[string]any) error {
	kind := str(v["type"])
	switch kind {
	case "text_delta":
		s.text(str(v["delta"]), false)
	case "thinking_delta":
		s.text(str(v["delta"]), true)
	case "toolcall_start":
		call := record(v["toolCall"])
		s.tool(int(integer(v["contentIndex"])), str(call["id"]), str(call["name"]), "")
	case "toolcall_delta":
		s.args(int(integer(v["contentIndex"])), str(v["delta"]))
	case "toolcall_end":
		call := record(v["toolCall"])
		key := int(integer(v["contentIndex"]))
		p := s.tool(key, str(call["id"]), str(call["name"]), "")
		if p.arguments.Len() == 0 {
			raw, _ := json.Marshal(record(call["arguments"]))
			p.arguments.Write(raw)
		}
		return s.endTool(key)
	case "done":
		s.terminal = true
		if msg := record(v["message"]); len(msg) > 0 {
			if u := record(msg["usage"]); len(u) > 0 {
				s.message.Usage = Usage{Input: integer(u["input"]), Output: integer(u["output"]), CacheRead: integer(u["cacheRead"]), CacheWrite: integer(u["cacheWrite"]), Known: true}
			}
		}
	case "error":
		return ErrInvalidResponse
	}
	return nil
}
