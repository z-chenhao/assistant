package provider

import (
	"encoding/json"
	"net/url"
	"strings"
)

func chatMessages(m Model, c Context) []any {
	out := []any{}
	if c.SystemPrompt != "" {
		role := "system"
		if m.Reasoning && boolCompat(m, "supportsDeveloperRole", true) {
			role = "developer"
		}
		out = append(out, map[string]any{"role": role, "content": c.SystemPrompt})
	}
	for i := 0; i < len(c.Messages); i++ {
		msg := c.Messages[i]
		if msg.Role == "toolResult" {
			images := []any{}
			for ; i < len(c.Messages) && c.Messages[i].Role == "toolResult"; i++ {
				result := c.Messages[i]
				wire := map[string]any{"role": "tool", "tool_call_id": strings.Split(result.ToolCallID, "|")[0], "content": messageText(result)}
				if boolCompat(m, "requiresToolResultName", false) {
					wire["name"] = result.ToolName
				}
				out = append(out, wire)
				for _, v := range result.Content {
					if v.Type == "image" {
						images = append(images, map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL(v)}})
					}
				}
			}
			i--
			if len(images) > 0 {
				out = append(out, map[string]any{"role": "user", "content": images})
			}
			continue
		}
		parts := []any{}
		calls := []any{}
		thinking := ""
		for _, v := range msg.Content {
			switch v.Type {
			case "text":
				parts = append(parts, map[string]any{"type": "text", "text": v.Text})
			case "image":
				parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL(v)}})
			case "thinking":
				thinking += v.Thinking
			case "toolCall":
				a, _ := json.Marshal(v.Arguments)
				calls = append(calls, map[string]any{"id": strings.Split(v.ID, "|")[0], "type": "function", "function": map[string]any{"name": v.Name, "arguments": string(a)}})
			}
		}
		var content any = parts
		if len(parts) == 0 {
			content = ""
		} else if len(parts) == 1 && parts[0].(map[string]any)["type"] == "text" {
			content = parts[0].(map[string]any)["text"]
		}
		wire := map[string]any{"role": msg.Role, "content": content}
		if len(calls) > 0 {
			wire["tool_calls"] = calls
		}
		if thinking != "" || boolCompat(m, "requiresReasoningContentOnAssistantMessages", false) && msg.Role == "assistant" {
			wire["reasoning_content"] = thinking
		}
		out = append(out, wire)
	}
	return out
}

func (b *requestBuilder) chat() {
	m := b.model
	c := b.context
	o := b.options
	base := b.base
	max := b.maxTokens
	p := b.payload
	if m.API == MistralConversations && !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	u, _ := url.Parse(base + "/chat/completions")
	p["messages"] = chatMessages(m, c)
	p[stringCompat(m, "maxTokensField", "max_completion_tokens")] = max
	if boolCompat(m, "supportsStore", true) {
		p["store"] = false
	}
	if boolCompat(m, "supportsUsageInStreaming", true) {
		p["stream_options"] = map[string]any{"include_usage": true}
	}
	if len(c.Tools) > 0 {
		tools := []any{}
		for _, t := range c.Tools {
			f := map[string]any{"name": t.Name, "description": t.Description, "parameters": object(t.Parameters)}
			if boolCompat(m, "supportsStrictMode", true) {
				f["strict"] = false
			}
			tools = append(tools, map[string]any{"type": "function", "function": f})
		}
		p["tools"] = tools
	}
	if o.Reasoning != "" {
		switch stringCompat(m, "thinkingFormat", "") {
		case "deepseek":
			p["thinking"] = map[string]any{"type": "enabled"}
			p["reasoning_effort"] = o.Reasoning
		case "qwen":
			p["enable_thinking"] = true
		case "zai":
			p["thinking"] = map[string]any{"type": "enabled"}
		default:
			if boolCompat(m, "supportsReasoningEffort", true) {
				p["reasoning_effort"] = o.Reasoning
			}
		}
	}
	if o.ToolChoice != "" {
		p["tool_choice"] = o.ToolChoice
	}

	b.payload = p
	b.url = u
}
