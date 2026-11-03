package coding

import "encoding/json"

// Command preserves the accepted action and actual terminal receipt. A failed,
// cancelled or unknown invocation never becomes a successful validation claim.
type Command struct {
	Command string          `json:"command"`
	Status  string          `json:"status"`
	Result  json.RawMessage `json:"result"`
}
type Report struct {
	Workspace
	Commands []Command `json:"commands"`
	Diff     *Diff     `json:"diff,omitempty"`
}
