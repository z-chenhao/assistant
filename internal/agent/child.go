package agent

import (
	"encoding/json"
	"strings"

	"github.com/z-chenhao/erii/internal/task"
	"github.com/z-chenhao/erii/internal/tool"
)

const RunToolName = "builtin__agent_run"
const RunToolSchema = `{"type":"object","properties":{"task":{"type":"string","minLength":1,"maxLength":8192},"artifactRefs":{"type":"array","maxItems":127,"items":{"type":"string","minLength":1,"maxLength":256}}},"required":["task"],"additionalProperties":false}`

// AgentRunRequest and AgentResult are the entire model-visible delegation API.
// Internal identities, claims, policies and usage never cross this boundary.
type AgentRunRequest struct {
	Task         string   `json:"task"`
	ArtifactRefs []string `json:"artifactRefs,omitempty"`
}
type AgentResult struct {
	Text         string   `json:"text"`
	ArtifactRefs []string `json:"artifactRefs"`
}

func AgentToolDefinition() tool.Definition {
	return tool.Definition{Host: true, Spec: tool.Spec{Name: RunToolName, Description: "Delegate explicit work to one general Worker. Returns only final text and artifactRefs to this Worker. Never creates a Task.", Version: 1, InputSchema: RunToolSchema, Permission: "spawn_agents", ReplayPolicy: "safe", Risk: "low", TimeoutMS: 10000, OutputBytes: 1 << 20, Provider: "builtin", ConflictPolicy: "none"}}
}

func ParseAgentRun(args string) (AgentRunRequest, error) {
	var req AgentRunRequest
	if !tool.Validate(RunToolSchema, args) || json.Unmarshal([]byte(args), &req) != nil || strings.TrimSpace(req.Task) == "" {
		return req, task.ErrInvalid
	}
	return req, nil
}

// WorkerTools is the fixed role reduction. New control permissions fail closed.
// This derives from frozen specs, never from the live registry or Task input.
func WorkerTools(specs []tool.Spec) []tool.Spec {
	out := []tool.Spec{}
	for _, s := range specs {
		if strings.HasPrefix(s.Name, "builtin__task_") {
			continue
		}
		switch s.Permission {
		case "read_only", "external_effect", "artifact_write", "spawn_agents", "automation_observe":
			out = append(out, s)
		}
	}
	return out
}

// ChildTools applies the additional fixed reduction for a Run tree descendant.
// Threshold state belongs to the Automation Root, never to its delegated work.
func ChildTools(specs []tool.Spec) []tool.Spec {
	out := []tool.Spec{}
	for _, s := range WorkerTools(specs) {
		if s.Permission != "automation_observe" {
			out = append(out, s)
		}
	}
	return out
}

// ChildLimits are Host configuration, frozen once per new Root tree.
// Scheduling concurrency is independent of these recursion safety ceilings.
type ChildLimits struct {
	Depth       int `yaml:"max_depth"`
	Descendants int `yaml:"max_descendants"`
	ToolCalls   int `yaml:"max_tool_calls"`
}

func DefaultChildLimits() ChildLimits {
	return ChildLimits{Depth: 8, Descendants: 4096, ToolCalls: 10000}
}
func (l ChildLimits) Valid() bool {
	return l.Depth >= 1 && l.Depth <= 16 && l.Descendants >= 1 && l.Descendants <= 10000 && l.ToolCalls >= 1 && l.ToolCalls <= 50000
}
