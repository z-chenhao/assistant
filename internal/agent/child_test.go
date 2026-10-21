package agent

import (
	"github.com/z-chenhao/erii/internal/tool"
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
		{Name: "builtin__task__search", Permission: "read_only"},
		{Name: "builtin__task__manage", Permission: "task_control"},
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
		if s != in[i+6] {
			t.Fatal("capability changed", got)
		}
	}
	if len(WorkerTools(nil)) != 0 {
		t.Fatal("capability introduced")
	}
}
