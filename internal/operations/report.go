// Package operations defines the bounded local operator read models.
package operations

import "github.com/z-chenhao/erii/internal/agent"

// Operations is an operator-only bounded projection. It deliberately excludes
// prompts, arguments, results, destinations, endpoint/account and credential refs.
type Report struct {
	Version int                   `json:"version"`
	At      string                `json:"at"`
	Counts  map[string]int64      `json:"counts"`
	Limits  agent.SchedulerLimits `json:"scheduler_limits"`
	Trees   []TreeHealth          `json:"trees"`
	DLQ     []DeadEvent           `json:"dlq"`
}
type TreeHealth struct {
	Root          string `json:"root_run_id"`
	Ticket        int64  `json:"last_served"`
	Live          int64  `json:"live_workers"`
	Queued        int64  `json:"queued_workers"`
	OldestSeconds int64  `json:"oldest_queued_seconds"`
}
type DeadEvent struct {
	ID       string `json:"event_id"`
	Attempts int    `json:"attempts"`
	Failures int    `json:"failures"`
	Code     string `json:"code"`
}

type TracePoint struct {
	Sequence int64  `json:"sequence"`
	Kind     string `json:"kind"`
	At       string `json:"occurred_at"`
	Tool     string `json:"tool_call_id,omitempty"`
}
type RunTrace struct {
	RunID  string       `json:"run_id"`
	Parent string       `json:"parent_run_id,omitempty"`
	Root   string       `json:"root_run_id,omitempty"`
	Status string       `json:"status"`
	Points []TracePoint `json:"points"`
}
