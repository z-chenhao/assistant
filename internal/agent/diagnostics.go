// Package agent owns the application readiness vocabulary. No Agent Loop or
// model connection is implemented by this foundation slice.
package agent

// Diagnostics is the local CLI's inspectable startup result, not AgentStatus.
type Diagnostics struct {
	Readiness            string  `json:"readiness"`
	AgentIngressEnabled  bool    `json:"agent_ingress_enabled"`
	AgentID              string  `json:"agent_id"`
	ActiveModelProfileID *string `json:"active_model_profile_id"`
	ConfigVersion        int64   `json:"config_version"`
	SchemaVersion        int     `json:"schema_version"`
	StateDatabase        string  `json:"state_database"`
}

const SetupRequired = "setup_required"
