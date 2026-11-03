package coding

import "encoding/json"

// Command preserves the accepted action and actual terminal receipt. A failed,
// cancelled or unknown invocation never becomes a successful validation claim.
type Command struct {
	Command    string          `json:"command"`
	AcceptedAt int64           `json:"accepted_at_unix_micro"`
	Attempts   int             `json:"attempts"`
	Error      json.RawMessage `json:"error"`
	Status     string          `json:"status"`
	Result     json.RawMessage `json:"result"`
}
type Report struct {
	Workspace
	CommandsTruncated bool      `json:"commands_truncated"`
	Commands          []Command `json:"commands"`
	Diff              *Diff     `json:"diff,omitempty"`
}
