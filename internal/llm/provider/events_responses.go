package provider

import (
	"encoding/json"
)

func (s *streamState) consumeResponses(api API, v map[string]any) error {
	kind := str(v["type"])
	key := int(integer(v["output_index"]))
	switch kind {
	case "response.created":
		s.message.ResponseID = str(record(v["response"])["id"])
	case "response.output_text.delta":
		s.text(str(v["delta"]), false)
	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		s.text(str(v["delta"]), true)
	case "response.output_item.added":
		item := record(v["item"])
		if item["type"] == "function_call" {
			p := s.tool(key, str(item["call_id"]), str(item["name"]), str(item["namespace"]))
			s.message.Content[p.index].ProviderID = str(item["id"])
		}
	case "response.function_call_arguments.delta":
		s.args(key, str(v["delta"]))
	case "response.output_item.done":
		item := record(v["item"])
		if item["type"] == "function_call" {
			p := s.tool(key, str(item["call_id"]), str(item["name"]), str(item["namespace"]))
			if p.arguments.Len() == 0 {
				p.arguments.WriteString(str(item["arguments"]))
			}
			if err := s.endTool(key); err != nil {
				return err
			}
		} else if item["type"] == "reasoning" {
			raw, _ := json.Marshal(item)
			found := false
			for i := len(s.message.Content) - 1; i >= 0; i-- {
				if s.message.Content[i].Type == "thinking" {
					s.message.Content[i].ThinkingSignature = string(raw)
					found = true
					break
				}
			}
			if !found {
				s.message.Content = append(s.message.Content, Content{Type: "thinking", ThinkingSignature: string(raw)})
			}
		} else if item["type"] == "message" {
			for i := len(s.message.Content) - 1; i >= 0; i-- {
				if s.message.Content[i].Type == "text" {
					signature := map[string]any{"v": 1, "id": str(item["id"])}
					if phase := str(item["phase"]); phase != "" {
						signature["phase"] = phase
					}
					raw, _ := json.Marshal(signature)
					s.message.Content[i].TextSignature = string(raw)
					break
				}
			}
		}
	case "response.completed", "response.incomplete":
		s.terminal = true
		response := record(v["response"])
		if kind == "response.incomplete" {
			if str(record(response["incomplete_details"])["reason"]) != "max_output_tokens" {
				return ErrInvalidResponse
			}
			s.message.StopReason = "length"
		}
		s.message.ResponseID = str(response["id"])
		if err := s.usage(record(response["usage"]), api); err != nil {
			return err
		}
	case "response.failed", "error":
		return ErrInvalidResponse
	}

	return nil
}
