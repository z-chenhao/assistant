package provider

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func responseMessages(c Context) []any {
	out := []any{}
	for msgIndex, m := range c.Messages {
		if m.Role == "toolResult" {
			out = append(out, map[string]any{"type": "function_call_output", "call_id": strings.Split(m.ToolCallID, "|")[0], "output": messageText(m)})
			images := []any{}
			for _, v := range m.Content {
				if v.Type == "image" {
					images = append(images, map[string]any{"type": "input_image", "image_url": imageURL(v), "detail": "auto"})
				}
			}
			if len(images) > 0 {
				out = append(out, map[string]any{"role": "user", "content": images})
			}
			continue
		}
		parts := []any{}
		flush := func() {
			if len(parts) > 0 {
				out = append(out, map[string]any{"role": m.Role, "content": parts})
				parts = []any{}
			}
		}
		for _, v := range m.Content {
			switch v.Type {
			case "text":
				if m.Role == "assistant" {
					flush()
					signature := map[string]any{}
					_ = json.Unmarshal([]byte(v.TextSignature), &signature)
					id := str(signature["id"])
					if id == "" {
						id = fmt.Sprintf("msg_steward_%d_%d", msgIndex, len(out))
					}
					item := map[string]any{"type": "message", "role": "assistant", "id": id, "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": v.Text, "annotations": []any{}}}}
					if phase := str(signature["phase"]); phase == "commentary" || phase == "final_answer" {
						item["phase"] = phase
					}
					out = append(out, item)
					continue
				}
				kind := "input_text"
				if m.Role == "assistant" {
					kind = "output_text"
				}
				parts = append(parts, map[string]any{"type": kind, "text": v.Text})
			case "image":
				parts = append(parts, map[string]any{"type": "input_image", "image_url": imageURL(v), "detail": "auto"})
			case "thinking":
				if v.ThinkingSignature != "" {
					flush()
					var item map[string]any
					if json.Unmarshal([]byte(v.ThinkingSignature), &item) == nil {
						out = append(out, item)
					}
				}
			case "toolCall":
				flush()
				a, _ := json.Marshal(v.Arguments)
				call := map[string]any{"type": "function_call", "call_id": strings.Split(v.ID, "|")[0], "name": v.Name, "arguments": string(a)}
				if v.ProviderID != "" {
					call["id"] = v.ProviderID
				}
				if v.Namespace != "" {
					call["namespace"] = v.Namespace
				}
				out = append(out, call)
			}
		}
		flush()
	}
	return out
}
func responsesTools(tools []Tool) []any {
	out := []any{}
	groups := map[string][]any{}
	order := []string{}
	for _, t := range tools {
		v := map[string]any{"type": "function", "name": t.Name, "description": t.Description, "parameters": object(t.Parameters), "strict": false}
		if t.Namespace == "" {
			out = append(out, v)
		} else {
			if _, ok := groups[t.Namespace]; !ok {
				order = append(order, t.Namespace)
			}
			groups[t.Namespace] = append(groups[t.Namespace], v)
		}
	}
	for _, name := range order {
		out = append(out, map[string]any{"type": "namespace", "name": name, "tools": groups[name]})
	}
	return out
}

func (b *requestBuilder) responses() {
	m := b.model
	c := b.context
	o := b.options
	base := b.base
	h := b.headers
	max := b.maxTokens
	key := b.key
	p := b.payload
	var u *url.URL
	if m.API == CodexResponses && strings.HasSuffix(base, "/codex") {
		u, _ = url.Parse(base + "/responses")
	} else if m.API == CodexResponses && !strings.HasSuffix(base, "/responses") {
		u, _ = url.Parse(base + "/codex/responses")
	} else if m.API == CodexResponses {
		u, _ = url.Parse(base)
	} else {
		u, _ = url.Parse(base + "/responses")
	}
	if m.API == AzureResponses {
		h.Del("Authorization")
		if o.AccessToken != "" {
			h.Set("Authorization", "Bearer "+o.AccessToken)
		} else {
			h.Set("api-key", o.APIKey)
		}
		if o.APIVersion != "" {
			q := u.Query()
			q.Set("api-version", o.APIVersion)
			u.RawQuery = q.Encode()
		}
		if o.Deployment != "" {
			p["model"] = o.Deployment
		}
	}
	p["input"] = responseMessages(c)
	if c.SystemPrompt != "" {
		p["instructions"] = c.SystemPrompt
	}
	p["store"] = false
	if m.API == CodexResponses {
		h.Set("OpenAI-Beta", "responses=experimental")
		h.Set("originator", "steward")
		if o.SessionID != "" {
			h.Set("session-id", o.SessionID)
			h.Set("x-client-request-id", o.SessionID)
		}
		parts := strings.Split(key, ".")
		if len(parts) == 3 {
			if raw, err := base64.RawURLEncoding.DecodeString(parts[1]); err == nil {
				var claims map[string]any
				if json.Unmarshal(raw, &claims) == nil {
					if id := str(record(claims["https://api.openai.com/auth"])["chatgpt_account_id"]); id != "" {
						h.Set("chatgpt-account-id", id)
					}
				}
			}
		}
		p["include"] = []string{"reasoning.encrypted_content"}
	}
	if boolCompat(m, "supportsMaxOutputTokens", true) {
		p["max_output_tokens"] = max
	}
	if len(c.Tools) > 0 {
		p["tools"] = responsesTools(c.Tools)
	}
	if o.Reasoning != "" && o.Reasoning != "off" {
		p["reasoning"] = map[string]any{"effort": o.Reasoning, "summary": "auto"}
		p["include"] = []string{"reasoning.encrypted_content"}
	}
	if o.ToolChoice != "" {
		p["tool_choice"] = o.ToolChoice
	}
	if o.SessionID != "" {
		p["prompt_cache_key"] = o.SessionID
	}
	if c.WebSearch {
		p["tools"] = []any{map[string]any{"type": "web_search"}}
		p["tool_choice"] = "required"
		if m.API != OpenAIResponses || m.Provider != "steward" {
			p["max_tool_calls"] = 1
		}
		p["include"] = []string{"web_search_call.action.sources"}
		if len(c.WebSearchPayload) > 0 {
			var body map[string]any
			if json.Unmarshal(c.WebSearchPayload, &body) == nil {
				p = body
			}
		}
	}

	b.payload = p
	b.url = u
}
