package agent

// SchedulerLimits are deployment policy, never model-visible capacity hints.
// The local data root has one user; User therefore also bounds all Workers.
type SchedulerLimits struct {
	Children               int `yaml:"child_concurrency"`
	Global                 int `yaml:"global_concurrency"`
	User                   int `yaml:"user_concurrency"`
	Agent                  int `yaml:"agent_concurrency"`
	Provider               int `yaml:"provider_concurrency"`
	Turns                  int `yaml:"max_worker_turns"`
	InfrastructureFailures int `yaml:"infrastructure_failures"`
	CooldownSeconds        int `yaml:"cooldown_seconds"`
}

func DefaultSchedulerLimits() SchedulerLimits {
	return SchedulerLimits{Children: 16, Global: 32, User: 32, Agent: 32, Provider: 32, Turns: 256, InfrastructureFailures: 5, CooldownSeconds: 30}
}

func (l SchedulerLimits) Valid() bool {
	for _, n := range []int{l.Children, l.Global, l.User, l.Agent, l.Provider} {
		if n < 1 || n > 32 {
			return false
		}
	}
	return l.Turns >= 1 && l.Turns <= 512 && l.InfrastructureFailures >= 2 && l.InfrastructureFailures <= 100 && l.CooldownSeconds >= 1 && l.CooldownSeconds <= 300
}
