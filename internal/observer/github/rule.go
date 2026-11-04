package github

// RuleDecision is owner-facing evidence of a rule evaluation, independent of
// whether Main later admitted or continued any work.
type RuleDecision struct {
	Sequence                int64  `json:"observation_sequence"`
	EventKey                string `json:"event_key"`
	Kind                    string `json:"event_kind"`
	ExpectedKind            string `json:"expected_event_kind"`
	ExpectedCheckConclusion string `json:"expected_check_conclusion,omitempty"`
	ActualCheckConclusion   string `json:"actual_check_conclusion,omitempty"`
	TaskRef                 string `json:"task_ref"`
	Generation              int64  `json:"rule_generation"`
	Mode                    string `json:"mode"`
	Matched                 bool   `json:"matched"`
	Reason                  string `json:"reason"`
}
