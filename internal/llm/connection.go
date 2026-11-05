package llm

import (
	"context"
	"time"
)

// Connection is the Host-owned, transient resolution result for one model call.
// Adapters serialize its credential fields; authorization guards stay in the Host.
// Never persist it in a profile, snapshot, checkpoint or diagnostic.
type Connection struct {
	Endpoint     string                      `json:"endpoint"`
	APIKey       string                      `json:"api_key,omitempty"`
	AccessToken  string                      `json:"access_token,omitempty"`
	ConnectionID string                      `json:"connection_id,omitempty"`
	AuthVersion  int64                       `json:"-"`
	AuthEpoch    int64                       `json:"-"`
	Guard        func(context.Context) error `json:"-"`
}
type ConnectionResolver func(context.Context, Snapshot) (Connection, error)
type GenerationObserver func(context.Context, GenerationRequest, Connection, GenerationResult, time.Duration, error) error
