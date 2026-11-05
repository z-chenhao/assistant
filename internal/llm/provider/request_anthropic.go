package provider

import (
	"net/url"
	"strings"
)

func anthropicMessages(c Context) []any {
	out := []any{}
	for _, m := range c.Messages {
		role := m.Role
		parts := []any{}
		if role == "toolResult" {
			role = "user"
			content := []any{map[string]any{"type": "text", "text": messageText(m)}}
			for _, v := range m.Content {
				if v.Type == "image" {
					content = append(content, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": v.MIMEType, "data": v.Data}})
				}
			}
			parts = append(parts, map[string]any{"type": "tool_result", "tool_use_id": strings.Split(m.ToolCallID, "|")[0], "content": content, "is_error": m.IsError})
		} else {
			for _, v := range m.Content {
				switch v.Type {
				case "text":
					parts = append(parts, map[string]any{"type": "text", "text": v.Text})
				case "image":
					parts = append(parts, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": v.MIMEType, "data": v.Data}})
				case "toolCall":
					parts = append(parts, map[string]any{"type": "tool_use", "id": v.ID, "name": v.Name, "input": v.Arguments})
				case "thinking":
					if v.Redacted {
						parts = append(parts, map[string]any{"type": "redacted_thinking", "data": v.ThinkingSignature})
					} else if v.ThinkingSignature != "" {
						parts = append(parts, map[string]any{"type": "thinking", "thinking": v.Thinking, "signature": v.ThinkingSignature})
					}
				}
			}
		}
		if len(parts) == 0 {
			continue
		}
		if len(out) > 0 && out[len(out)-1].(map[string]any)["role"] == role {
			last := out[len(out)-1].(map[string]any)
			last["content"] = append(last["content"].([]any), parts...)
		} else {
			out = append(out, map[string]any{"role": role, "content": parts})
		}
	}
	return out
}

func (b *requestBuilder) anthropic() {
	m := b.model
	c := b.context
	o := b.options
	base := b.base
	h := b.headers
	max := b.maxTokens
	key := b.key
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	u, _ := url.Parse(base + "/messages")
	h.Del("Authorization")
	h.Set("anthropic-version", "2023-06-01")
	if o.AccessToken != "" {
		h.Set("Authorization", "Bearer "+o.AccessToken)
	} else if key != "" {
		h.Set("x-api-key", key)
	}
	p := map[string]any{"model": m.ID, "stream": true, "max_tokens": max, "messages": anthropicMessages(c)}
	if c.SystemPrompt != "" {
		system := map[string]any{"type": "text", "text": c.SystemPrompt}
		if o.CacheRetention != "none" {
			system["cache_control"] = cacheControl(o.CacheRetention)
		}
		p["system"] = []any{system}
	}
	if len(c.Tools) > 0 {
		tools := []any{}
		for _, t := range c.Tools {
			tools = append(tools, map[string]any{"name": t.Name, "description": t.Description, "input_schema": object(t.Parameters)})
		}
		if o.CacheRetention != "none" {
			tools[len(tools)-1].(map[string]any)["cache_control"] = cacheControl(o.CacheRetention)
		}
		p["tools"] = tools
	}
	if o.Reasoning != "" && o.Reasoning != "off" {
		budget := o.ThinkingBudget
		if budget == 0 {
			budget = map[string]int64{"minimal": 1024, "low": 2048, "medium": 8192, "high": 16384, "max": 32768}[o.Reasoning]
		}
		if budget == 0 {
			budget = 8192
		}
		if boolCompat(m, "forceAdaptiveThinking", false) {
			p["thinking"] = map[string]any{"type": "adaptive"}
			p["output_config"] = map[string]any{"effort": o.Reasoning}
		} else {
			p["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget}
		}
		if !boolCompat(m, "forceAdaptiveThinking", false) && max <= budget {
			p["max_tokens"] = budget + 4096
		}
	}
	if o.ToolChoice != "" {
		p["tool_choice"] = map[string]any{"type": o.ToolChoice}
	}

	b.payload = p
	b.url = u
}
