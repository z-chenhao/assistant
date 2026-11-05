package provider

import (
	"net/url"
)

func (b *requestBuilder) gateway() {
	in := b.original
	m := b.model
	o := b.options
	base := b.base
	max := b.maxTokens
	u, _ := url.Parse(base + "/messages")
	p := map[string]any{"model": m.ID, "context": in, "options": map[string]any{"maxTokens": max, "reasoning": o.Reasoning, "sessionId": o.SessionID, "cacheRetention": o.CacheRetention, "toolChoice": o.ToolChoice}}
	b.payload = p
	b.url = u
}
