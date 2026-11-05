package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Mistral uses its own chat fields and nine-character alphanumeric call IDs.
// Keep the mapping local to a request and consistent across calls and results.
func mistralMessages(c Context) []any {
	ids, owners := map[string]string{}, map[string]string{}
	normalize := func(id string) string {
		if mapped := ids[id]; mapped != "" {
			return mapped
		}
		clean := strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, id)
		for attempt := 0; ; attempt++ {
			candidate := clean
			if len(candidate) != 9 || attempt > 0 {
				hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", id, attempt)))
				candidate = hex.EncodeToString(hash[:])[:9]
			}
			if owner, found := owners[candidate]; !found || owner == id {
				ids[id], owners[candidate] = candidate, id
				return candidate
			}
		}
	}
	out := []any{}
	if c.SystemPrompt != "" {
		out = append(out, map[string]any{"role": "system", "content": c.SystemPrompt})
	}
	for _, message := range c.Messages {
		parts, calls := []any{}, []any{}
		for _, part := range message.Content {
			switch part.Type {
			case "text":
				parts = append(parts, map[string]any{"type": "text", "text": part.Text})
			case "image":
				parts = append(parts, map[string]any{"type": "image_url", "image_url": imageURL(part)})
			case "thinking":
				parts = append(parts, map[string]any{"type": "thinking", "thinking": []any{map[string]any{"type": "text", "text": part.Thinking}}})
			case "toolCall":
				args, _ := json.Marshal(part.Arguments)
				calls = append(calls, map[string]any{"id": normalize(part.ID), "type": "function", "function": map[string]any{"name": part.Name, "arguments": string(args)}})
			}
		}
		role := message.Role
		wire := map[string]any{"role": role, "content": parts}
		if role == "toolResult" {
			wire["role"], wire["tool_call_id"], wire["name"] = "tool", normalize(message.ToolCallID), message.ToolName
		}
		if role == "assistant" {
			wire["prefix"] = false
			if len(calls) > 0 {
				wire["tool_calls"] = calls
			}
		}
		out = append(out, wire)
	}
	return out
}

func (b *requestBuilder) mistral() {
	base := strings.TrimRight(b.base, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	b.url, _ = url.Parse(base + "/chat/completions")
	p := map[string]any{"model": b.model.ID, "stream": true, "messages": mistralMessages(b.context), "max_tokens": b.maxTokens}
	if len(b.context.Tools) > 0 {
		tools := []any{}
		for _, t := range b.context.Tools {
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": t.Name, "description": t.Description, "parameters": object(t.Parameters), "strict": false}})
		}
		p["tools"] = tools
	}
	if choice := b.options.ToolChoice; choice != "" {
		if choice == "required" {
			choice = "any"
		}
		p["tool_choice"] = choice
	}
	if b.model.Reasoning {
		if len(b.model.ThinkingLevelMap) > 0 {
			if effort, ok := b.model.ThinkingLevelMap[b.options.Reasoning].(string); ok {
				p["reasoning_effort"] = effort
			}
		} else if b.options.Reasoning != "" && b.options.Reasoning != "off" {
			p["prompt_mode"] = "reasoning"
		}
	}
	if b.options.SessionID != "" && b.options.CacheRetention != "none" {
		p["prompt_cache_key"] = b.options.SessionID
	}
	b.payload = p
}
