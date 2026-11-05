package provider

import (
	"net/url"
	"strings"
)

func bedrockMessages(c Context) []any {
	out := []any{}
	for _, m := range c.Messages {
		role := m.Role
		parts := []any{}
		if role == "toolResult" {
			role = "user"
			status := "success"
			if m.IsError {
				status = "error"
			}
			content := []any{map[string]any{"text": messageText(m)}}
			for _, v := range m.Content {
				if v.Type == "image" {
					content = append(content, map[string]any{"image": map[string]any{"format": strings.TrimPrefix(v.MIMEType, "image/"), "source": map[string]any{"bytes": v.Data}}})
				}
			}
			parts = append(parts, map[string]any{"toolResult": map[string]any{"toolUseId": m.ToolCallID, "status": status, "content": content}})
		} else {
			for _, v := range m.Content {
				switch v.Type {
				case "text":
					parts = append(parts, map[string]any{"text": v.Text})
				case "image":
					format := strings.TrimPrefix(v.MIMEType, "image/")
					if format == "jpeg" {
						format = "jpeg"
					}
					parts = append(parts, map[string]any{"image": map[string]any{"format": format, "source": map[string]any{"bytes": v.Data}}})
				case "toolCall":
					parts = append(parts, map[string]any{"toolUse": map[string]any{"toolUseId": v.ID, "name": v.Name, "input": v.Arguments}})
				case "thinking":
					if v.ThinkingSignature != "" {
						parts = append(parts, map[string]any{"reasoningContent": map[string]any{"reasoningText": map[string]any{"text": v.Thinking, "signature": v.ThinkingSignature}}})
					}
				}
			}
		}
		if len(parts) > 0 {
			out = append(out, map[string]any{"role": role, "content": parts})
		}
	}
	return out
}

func (b *requestBuilder) bedrock() {
	m := b.model
	c := b.context
	o := b.options
	base := b.base
	h := b.headers
	max := b.maxTokens
	key := b.key
	u, _ := url.Parse(base + "/model/" + url.PathEscape(m.ID) + "/converse-stream")
	if key == "" {
		h.Del("Authorization")
	}
	h.Set("Accept", "application/vnd.amazon.eventstream")
	p := map[string]any{"messages": bedrockMessages(c), "inferenceConfig": map[string]any{"maxTokens": max}}
	if c.SystemPrompt != "" {
		p["system"] = []any{map[string]any{"text": c.SystemPrompt}}
	}
	if o.Temperature != nil {
		p["inferenceConfig"].(map[string]any)["temperature"] = *o.Temperature
	}
	if len(c.Tools) > 0 {
		tools := []any{}
		for _, t := range c.Tools {
			tools = append(tools, map[string]any{"toolSpec": map[string]any{"name": t.Name, "description": t.Description, "inputSchema": map[string]any{"json": object(t.Parameters)}}})
		}
		p["toolConfig"] = map[string]any{"tools": tools}
	}
	if o.Reasoning != "" {
		budget := o.ThinkingBudget
		if budget == 0 {
			budget = 8192
		}
		p["additionalModelRequestFields"] = map[string]any{"thinking": map[string]any{"type": "enabled", "budget_tokens": budget}}
	}

	b.payload = p
	b.url = u
}
