// Package agent owns readiness, Actor coordination and the reusable Agent Loop.
package agent

import (
	"context"
)

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
const Ready = "ready"
const Degraded = "degraded"

type ModelAvailability interface {
	Available(context.Context, string) bool
}
type ProviderHealth interface {
	Health(context.Context) error
}

// EffectiveReadiness is evaluated after persistence, credentials and Bridge
// initialization. Persisted intent alone never enables ingress.
func EffectiveReadiness(ctx context.Context, d Diagnostics, models ModelAvailability, provider ProviderHealth) Diagnostics {
	if d.ActiveModelProfileID != nil {
		d.Readiness = Degraded
		if models != nil && provider != nil && models.Available(ctx, *d.ActiveModelProfileID) {
			if err := provider.Health(ctx); err == nil {
				d.Readiness = Ready
			}
		}
	}
	d.AgentIngressEnabled = d.Readiness == Ready
	return d
}
