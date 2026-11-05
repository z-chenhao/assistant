package agent

import (
	"github.com/z-chenhao/assistant/internal/tool"
	"reflect"
	"testing"
)

func TestAgentRunSchemaHasOnlyTaskAndArtifactRefs(t *testing.T) {
	for _, raw := range []string{`{"task":"work"}`, `{"task":"work","artifactRefs":[]}`, `{"task":"work","artifactRefs":["opaque"]}`} {
		if _, err := ParseAgentRun(raw); err != nil {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{`{}`, `{"task":null}`, `{"task":" "}`, `{"task":"a","task":"b"}`, `{"task":"work","artifactRefs":null}`, `{"task":"work","artifactRefs":[null]}`} {
		if _, err := ParseAgentRun(raw); err == nil {
			t.Fatal("accepted invalid request", raw)
		}
	}
	for _, key := range []string{"agent", "model", "skill", "tools", "permission", "schema", "timeout", "label", "replayKey", "call_key", "parent_run_id", "task_id"} {
		raw := `{"task":"work","` + key + `":"forged"}`
		if _, err := ParseAgentRun(raw); err == nil {
			t.Fatal("accepted control parameter", key)
		}
	}
}
func TestChildFixedReductionDoesNotAddCapabilities(t *testing.T) {
	in := []tool.Spec{
		{Name: "builtin__task_search", Permission: "read_only"},
		{Name: "builtin__task_manage", Permission: "task_control"},
		{Name: "credential", Permission: "credential_control"},
		{Name: "memory", Permission: "memory_write"},
		{Name: "delivery", Permission: "user_delivery"},
		{Name: "conversation", Permission: "conversation_control"},
		{Name: "read", Permission: "read_only"},
		{Name: "effect", Permission: "external_effect"},
		{Name: "artifact", Permission: "artifact_write"},
		{Name: RunToolName, Permission: "spawn_agents"},
	}
	got := WorkerTools(in)
	if len(got) != 4 {
		t.Fatal(got)
	}
	for i, s := range got {
		if !reflect.DeepEqual(s, in[i+6]) {
			t.Fatal("capability changed", got)
		}
	}
	if len(WorkerTools(nil)) != 0 {
		t.Fatal("capability introduced")
	}
}

func TestWebAccessRoleReductionUsesOnlyFrozenTools(t *testing.T) {
	input := []tool.Spec{{Name: "builtin__web_search", Permission: "read_only", ResourceHash: "frozen"}, {Name: "builtin__web_fetch", Permission: "read_only", ResourceHash: "frozen"}}
	for _, reduce := range []func([]tool.Spec) []tool.Spec{MainTools, WorkerTools, ChildTools} {
		if !reflect.DeepEqual(reduce(input), input) || len(reduce(nil)) != 0 {
			t.Fatal("web access widened or changed frozen tools")
		}
	}
}

func TestEvidenceConversationNavigationRemainsMainOnly(t *testing.T) {
	input := []tool.Spec{{Name: "builtin__evidence_read", Permission: "read_only"}, {Name: "builtin__read", Permission: "read_only"}}
	for _, reduced := range [][]tool.Spec{WorkerTools(input), ChildTools(input)} {
		if len(reduced) != 1 || reduced[0].Name != "builtin__read" {
			t.Fatal("Worker gained Main conversation navigation", reduced)
		}
	}
}

func TestMainToolsAllowConfiguredBusinessAndExcludeWorkerExecution(t *testing.T) {
	allowed := []tool.Spec{
		{Name: "builtin__codemode_execute", Permission: "read_only"},
		{Name: "builtin__task_manage", Permission: "task_control"},
		{Name: "builtin__evidence_read", Permission: "read_only"},
		{Name: "builtin__artifact_manage", Permission: "artifact_write"},
		{Name: "builtin__skill_list", Permission: "read_only"},
		{Name: "builtin__skill_view", Permission: "read_only"},
		{Name: "mcp__calendar__lookup", Permission: "read_only"},
		{Name: "mcp__calendar__create", Permission: "external_effect"},
	}
	specs := append([]tool.Spec{}, allowed...)
	for _, name := range []string{
		"builtin__read", "builtin__bash", "builtin__edit", "builtin__write",
		"builtin__computer_discover", "builtin__computer_bind", "builtin__computer_invoke",
		RunToolName, "builtin__automation_observe", "builtin__new_control",
	} {
		for _, permission := range []string{"read_only", "external_effect"} {
			specs = append(specs, tool.Spec{Name: name, Permission: permission})
		}
	}
	for _, permission := range []string{"task_control", "spawn_agents", "artifact_write", ""} {
		specs = append(specs, tool.Spec{Name: "mcp__calendar__unexpected", Permission: permission})
	}
	specs = append(specs, tool.Spec{Name: "mcp__private__bridge", Permission: "read_only", Internal: true})
	specs = append(specs, tool.Spec{Name: "builtin__memory_search", Permission: "read_only", Internal: true})
	if got := MainTools(specs); !reflect.DeepEqual(got, allowed) {
		t.Fatalf("Main business policy or Worker execution boundary changed: %+v", got)
	}
	if got := MainTools(nil); len(got) != 0 {
		t.Fatal("Main introduced a capability", got)
	}
}
