package llm

import "encoding/json"

// Dispatch is optional diagnostic evidence from the final HTTP boundary. It is
// neither an accepted generation nor a replay key. At is UTC Unix microseconds.
type Dispatch struct {
	GenerationID string          `json:"generation_id"`
	Attempt      string          `json:"attempt"`
	Phase        string          `json:"phase"`
	At           int64           `json:"at"`
	Format       string          `json:"format"`
	Body         json.RawMessage `json:"body,omitempty"`
	BodyHash     string          `json:"body_hash,omitempty"`
	Omitted      string          `json:"omitted,omitempty"`
	HTTPStatus   int             `json:"http_status,omitempty"`
	Error        string          `json:"error_code,omitempty"`
}
