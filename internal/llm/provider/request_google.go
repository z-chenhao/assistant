package provider

import (
	"net/url"
	"strings"
)

func googleMessages(c Context) []any {
	out := []any{}
	for _, m := range c.Messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		parts := []any{}
		if role == "toolResult" {
			role = "user"
			parts = append(parts, map[string]any{"functionResponse": map[string]any{"name": m.ToolName, "id": m.ToolCallID, "response": map[string]any{"output": messageText(m)}}})
			for _, v := range m.Content {
				if v.Type == "image" {
					parts = append(parts, map[string]any{"inlineData": map[string]any{"mimeType": v.MIMEType, "data": v.Data}})
				}
			}
		} else {
			for _, v := range m.Content {
				p := map[string]any{}
				switch v.Type {
				case "text":
					p["text"] = v.Text
				case "thinking":
					if v.ThinkingSignature == "" {
						continue
					}
					p["text"] = v.Thinking
					p["thought"] = true
					p["thoughtSignature"] = v.ThinkingSignature
				case "image":
					p["inlineData"] = map[string]any{"mimeType": v.MIMEType, "data": v.Data}
				case "toolCall":
					p["functionCall"] = map[string]any{"name": v.Name, "args": v.Arguments, "id": v.ID}
					if v.ThinkingSignature != "" {
						p["thoughtSignature"] = v.ThinkingSignature
					}
				default:
					continue
				}
				parts = append(parts, p)
			}
		}
		if len(parts) > 0 {
			if len(out) > 0 && out[len(out)-1].(map[string]any)["role"] == role {
				last := out[len(out)-1].(map[string]any)
				last["parts"] = append(last["parts"].([]any), parts...)
			} else {
				out = append(out, map[string]any{"role": role, "parts": parts})
			}
		}
	}
	return out
}

func (b *requestBuilder) google() {
	m := b.model
	c := b.context
	o := b.options
	base := b.base
	h := b.headers
	max := b.maxTokens
	key := b.key
	if m.API == GoogleVertex {
		project := o.Project
		location := o.Location
		if location == "" {
			location = "us-central1"
		}
		if project != "" {
			host := base
			endpoint, _ := url.Parse(base)
			if endpoint != nil && endpoint.Scheme == "https" && (endpoint.Hostname() == "aiplatform.googleapis.com" || strings.HasSuffix(endpoint.Hostname(), "-aiplatform.googleapis.com")) && (endpoint.Path == "" || endpoint.Path == "/v1") {
				host = "https://" + location + "-aiplatform.googleapis.com/v1"
				if location == "global" {
					host = "https://aiplatform.googleapis.com/v1"
				}
			}
			base = host + "/projects/" + url.PathEscape(project) + "/locations/" + url.PathEscape(location) + "/publishers/google"
		}
		if project == "" && o.APIKey != "" {
			base += "/publishers/google"
			h.Del("Authorization")
			h.Set("x-goog-api-key", o.APIKey)
		}
	} else {
		h.Del("Authorization")
		if key != "" {
			h.Set("x-goog-api-key", key)
		}
	}
	u, _ := url.Parse(base + "/models/" + url.PathEscape(m.ID) + ":streamGenerateContent?alt=sse")
	config := map[string]any{"maxOutputTokens": max}
	if o.Temperature != nil {
		config["temperature"] = *o.Temperature
	}
	if o.Reasoning != "" {
		thinking := map[string]any{"includeThoughts": true}
		if o.ThinkingBudget != 0 {
			thinking["thinkingBudget"] = o.ThinkingBudget
		} else {
			thinking["thinkingLevel"] = strings.ToUpper(o.Reasoning)
		}
		config["thinkingConfig"] = thinking
	}
	p := map[string]any{"contents": googleMessages(c), "generationConfig": config}
	if c.SystemPrompt != "" {
		p["systemInstruction"] = map[string]any{"parts": []any{map[string]any{"text": c.SystemPrompt}}}
	}
	if len(c.Tools) > 0 {
		decls := []any{}
		for _, t := range c.Tools {
			decls = append(decls, map[string]any{"name": t.Name, "description": t.Description, "parameters": object(t.Parameters)})
		}
		p["tools"] = []any{map[string]any{"functionDeclarations": decls}}
	}
	if o.ToolChoice != "" {
		mode := "AUTO"
		if o.ToolChoice == "none" {
			mode = "NONE"
		}
		p["toolConfig"] = map[string]any{"functionCallingConfig": map[string]any{"mode": mode}}
	}

	b.payload = p
	b.url = u
}
