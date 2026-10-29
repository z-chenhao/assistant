package bridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/z-chenhao/collie/internal/llm"
)

// SetDispatchObserver installs a nonblocking Host diagnostic sink. It cannot
// accept/reject model output and has no credentials or provider response body.
func (c *Client) SetDispatchObserver(sink func(llm.GenerationRequest, llm.Dispatch)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dispatchObserver = sink
}

func (s *eventStream) observe(raw json.RawMessage) {
	defer func() { _ = recover() }() // diagnostics cannot reject a model generation
	if s.capture == nil || s.observed >= 16 || s.terminal {
		return
	}
	var d llm.Dispatch
	if json.Unmarshal(raw, &d) != nil || d.GenerationID != s.id || len(d.Attempt) == 0 || len(d.Attempt) > 128 || d.At <= 0 || len(d.Body) > 256<<10 || len(d.Format) > 100 || len(d.Omitted) > 100 || len(d.BodyHash) > 64 {
		return
	}
	for _, secret := range s.wireSecrets {
		if secret != "" && (strings.Contains(d.Attempt, secret) || strings.Contains(d.Format, secret) || strings.Contains(d.Omitted, secret) || strings.Contains(d.BodyHash, secret)) {
			return
		}
	}
	if d.Phase != "started" && d.Phase != "headers" && d.Phase != "error" {
		return
	}
	if d.HTTPStatus < 0 || d.HTTPStatus > 599 || d.Body != nil && !json.Valid(d.Body) {
		return
	}
	if d.Error != "" {
		d.Error = llm.NormalizeError(d.Error).Error()
	}
	// The Host repeats credential removal instead of trusting the diagnostic
	// payload from the peer. Header/connection envelopes are never recorded.
	if d.Body != nil {
		var value any
		if json.Unmarshal(d.Body, &value) != nil {
			return
		}
		value = redactWireValue(value, s.wireSecrets)
		d.Body, _ = json.Marshal(value)
		hash := sha256.Sum256(d.Body)
		d.BodyHash = hex.EncodeToString(hash[:])
		if len(d.Body) > 256<<10 {
			d.Body = nil
			d.Omitted = "Host sanitized body exceeds 256 KiB"
		}
	}
	s.observed++
	s.capture(d)
}

func redactWireValue(value any, secrets []string) any {
	switch v := value.(type) {
	case string:
		for _, secret := range secrets {
			if secret != "" {
				v = strings.ReplaceAll(v, secret, "[credential redacted]")
			}
		}
		if strings.HasPrefix(v, "data:") {
			return "[media omitted]"
		}
		return v
	case []any:
		for i, x := range v {
			v[i] = redactWireValue(x, secrets)
		}
	case map[string]any:
		for k, x := range v {
			switch strings.ToLower(k) {
			case "authorization", "api_key", "access_token", "refresh_token", "cookie", "set-cookie":
				v[k] = "[credential redacted]"
			default:
				v[k] = redactWireValue(x, secrets)
			}
		}
	}
	return value
}
