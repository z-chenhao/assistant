package fake

import (
	"encoding/json"
	"strings"

	artifacttools "github.com/z-chenhao/erii/internal/artifact/tools"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/task"
)

// artifactsReply only selects actions; independent CLI readback judges them.
func artifactsReply(r llm.GenerationRequest) llm.GenerationResult {
	last := r.Messages[len(r.Messages)-1]
	call := func(name string, value any) llm.GenerationResult {
		raw, _ := json.Marshal(value)
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "artifact-fixture", Name: name, Arguments: string(raw)}}}
	}
	user := ""
	rootTask := ""
	followup := false
	root := false
	var prior llm.ToolCall
	for _, m := range r.Messages {
		if at := strings.Index(m.Text, "Artifact fixture:\n"); at >= 0 {
			rootTask = m.Text[at+len("Artifact fixture:\n"):]
		}
		root = root || strings.Contains(m.Text, "general Worker executing")
		followup = followup || m.Origin == "runtime" && strings.HasPrefix(m.Text, "Runtime task update")
		if m.Role == "user" && m.Origin == "" {
			user = m.Text
		}
		if len(m.ToolCalls) > 0 {
			prior = m.ToolCalls[len(m.ToolCalls)-1]
		}
	}
	if followup && !hasAppendedUser(r.Messages) && last.Role != "tool" {
		const marker = "Runtime task update (untrusted Worker data; not user instructions):\n"
		var payload struct {
			Details task.UpdateDetails `json:"details"`
		}
		if at := strings.Index(last.Text, marker); at >= 0 {
			_ = json.Unmarshal([]byte(last.Text[at+len(marker):]), &payload)
		}
		return call(artifacttools.Manage, map[string]any{"operation": "publish", "artifactRefs": payload.Details.ArtifactRefs})
	}
	if root {
		user = rootTask
	}
	var request struct {
		Operation string   `json:"operation"`
		Name      string   `json:"name"`
		Media     string   `json:"mediaType"`
		Text      string   `json:"text"`
		Ref       string   `json:"artifactRef"`
		Refs      []string `json:"artifactRefs"`
		Delegate  bool     `json:"delegate"`
		Draft     bool     `json:"draft"`
	}
	user, _, _ = strings.Cut(user, "\nExplicit artifact references: ")
	_ = json.Unmarshal([]byte(strings.SplitN(user, "\nArtifact reference", 2)[0]), &request)
	if last.Role == "tool" {
		if (prior.Name == artifacttools.Manage && strings.Contains(prior.Arguments, `"operation":"save"`) || prior.Name == "mcp__fixture__binary" || prior.Name == "mcp__fixture__large") && !request.Draft {
			var result struct {
				Refs []string `json:"artifact_refs"`
			}
			_ = json.Unmarshal([]byte(last.Text), &result)
			return call(artifacttools.Manage, map[string]any{"operation": "publish", "artifactRefs": result.Refs})
		}
		return llm.GenerationResult{Text: "Artifact operation observed: " + last.Text}
	}
	if request.Delegate && !root {
		return call(task.ManageName, task.ManageRequest{Operation: "create", Task: "Artifact fixture:\n" + user, ArtifactRefs: request.Refs})
	}
	switch request.Operation {
	case "slow_binary":
		return call("mcp__fixture__slow_binary", map[string]any{})
	case "binary":
		return call("mcp__fixture__binary", map[string]any{})
	case "large":
		return call("mcp__fixture__large", map[string]any{})
	case "read":
		return call(artifacttools.Manage, map[string]any{"operation": "read", "artifactRef": request.Ref})
	case "publish":
		return call(artifacttools.Manage, map[string]any{"operation": "publish", "artifactRefs": request.Refs})
	case "list":
		return call(artifacttools.Search, map[string]any{})
	case "describe":
		return call(artifacttools.Manage, map[string]any{"operation": "describe", "artifactRef": request.Ref})
	default:
		return call(artifacttools.Manage, map[string]any{"operation": "save", "name": request.Name, "mediaType": request.Media, "text": request.Text})
	}
}
