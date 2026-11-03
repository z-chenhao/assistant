package computer

// Activity is a bounded operator projection. Arguments, handle tokens, native
// identities and tool output stay in their existing execution stores.
type Activity struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	Status string `json:"status"`
	At     string `json:"at"`
}
