package provider

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type Request struct {
	URL     string
	Headers http.Header
	Body    map[string]any
}

func BuildRequest(m Model, in Context, o Options) (Request, error) {
	o = o.Resolve(m)
	m = resolveModel(m, o)
	c := Normalize(in)
	base := strings.TrimRight(m.BaseURL, "/")
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" {
		return Request{}, ErrIncompatible
	}
	h := http.Header{"Content-Type": []string{"application/json"}, "Accept": []string{"text/event-stream"}}
	key := o.APIKey
	if o.AccessToken != "" {
		key = o.AccessToken
	}
	if key != "" {
		h.Set("Authorization", "Bearer "+key)
	}
	for k, v := range m.Headers {
		h.Set(k, v)
	}
	for k, v := range o.Headers {
		h.Set(k, v)
	}
	if o.SessionID != "" && boolCompat(m, "sendSessionAffinityHeaders", true) {
		h.Set("session_id", o.SessionID)
	}
	max := o.MaxTokens
	if max == 0 {
		max = m.MaxTokens
	}
	if max <= 0 {
		max = 4096
	}
	p := map[string]any{"model": m.ID, "stream": true}
	b := requestBuilder{model: m, context: c, original: in, options: o, base: base, headers: h, maxTokens: max, key: key, payload: p, url: u}
	switch m.API {
	case OpenAICompletions, MistralConversations:
		b.chat()
	case OpenAIResponses, AzureResponses, CodexResponses:
		b.responses()
	case AnthropicMessages:
		b.anthropic()
	case GoogleGenerative, GoogleVertex:
		b.google()
	case BedrockConverse:
		b.bedrock()
	case GatewayMessages:
		b.gateway()
	default:
		return Request{}, ErrIncompatible
	}
	p, u = b.payload, b.url
	if m.Provider == "github-copilot" {
		if m.API == AnthropicMessages {
			h.Del("x-api-key")
			h.Set("Authorization", "Bearer "+key)
		}
		h.Set("Openai-Intent", "conversation-edits")
		initiator := "user"
		if len(c.Messages) > 0 && c.Messages[len(c.Messages)-1].Role != "user" {
			initiator = "agent"
		}
		h.Set("X-Initiator", initiator)
		for _, message := range c.Messages {
			for _, part := range message.Content {
				if part.Type == "image" {
					h.Set("Copilot-Vision-Request", "true")
				}
			}
		}
	}
	if (m.Provider == "opencode" || m.Provider == "opencode-go") && o.SessionID != "" && h.Get("x-opencode-session") == "" {
		h.Set("x-opencode-session", o.SessionID)
	}
	if m.Provider == "cloudflare-ai-gateway" {
		h.Del("Authorization")
		h.Del("x-api-key")
		h.Set("cf-aig-authorization", "Bearer "+key)
	}
	if m.API == AnthropicMessages && o.AccessToken != "" {
		h.Set("anthropic-beta", "claude-code-20250219,oauth-2025-04-20")
	}
	if o.Temperature != nil && boolCompat(m, "supportsTemperature", true) && m.API != GoogleGenerative && m.API != GoogleVertex && m.API != BedrockConverse {
		p["temperature"] = *o.Temperature
	}
	if len(o.Metadata) > 0 {
		p["metadata"] = o.Metadata
	}
	for k, v := range m.SamplingParams {
		p[k] = v
	}
	for k, v := range o.SamplingParams {
		p[k] = v
	}
	for k, v := range o.Extra {
		p[k] = v
	}
	if o.OnPayload != nil {
		if err := o.OnPayload(p); err != nil {
			return Request{}, err
		}
	}
	return Request{URL: u.String(), Headers: h, Body: p}, nil
}

func cacheControl(retention string) any {
	value := map[string]any{"type": "ephemeral"}
	if retention == "long" {
		value["ttl"] = "1h"
	}
	return value
}
func imageURL(c Content) string { return "data:" + c.MIMEType + ";base64," + c.Data }
func ValidateModel(m Model) error {
	for _, api := range APIs() {
		if api == m.API && m.ID != "" {
			return nil
		}
	}
	return fmt.Errorf("%w: unknown API", ErrIncompatible)
}

type requestBuilder struct {
	model     Model
	context   Context
	original  Context
	options   Options
	base      string
	headers   http.Header
	maxTokens int64
	key       string
	payload   map[string]any
	url       *url.URL
}
