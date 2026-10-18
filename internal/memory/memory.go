// Package memory owns derived durable user facts; Conversation owns originals.
package memory

import "time"

type SourceRef struct {
	Type string `json:"source_type"`
	Ref  string `json:"source_ref"`
}
type Record struct {
	ID         string      `json:"memory_id"`
	UserID     string      `json:"user_id"`
	ScopeType  string      `json:"scope_type"`
	ScopeID    string      `json:"scope_id"`
	Tier       string      `json:"tier"`
	Content    string      `json:"content"`
	SourceRefs []SourceRef `json:"source_refs"`
	Supersedes string      `json:"supersedes_memory_id,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
}
