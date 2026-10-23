package codemode

import (
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/tool"
)

func TestDescriptionIncludesSchemaDeclarationsAndInstructions(t *testing.T) {
	spec := tool.Spec{Name: "builtin__agent__run", Description: "Create one Child, never a Task.", InputSchema: `{"type":"object","properties":{"task":{"type":"string"},"artifactRefs":{"type":"array","items":{"type":"string"}}},"required":["task"],"additionalProperties":false}`}
	d := Describe(spec)
	for _, fragment := range []string{`"task": string`, `"artifactRefs"?: ReadonlyArray<string>`, `readonly artifactRefs: readonly string[]`} {
		if !strings.Contains(d.Declaration, fragment) {
			t.Fatal(d.Declaration)
		}
	}
	if d.InputSchema != spec.InputSchema || d.Instructions != spec.Description {
		t.Fatal(d)
	}
	if !NamespaceMatches("mcp__fixture__echo", "fixture") || !NamespaceMatches("mcp__mail__read", "mcp__mail") || NamespaceMatches("mcp__mail__read", "agent") {
		t.Fatal("namespace mapping")
	}
}

func TestDefinitionExplainsUnifiedCapabilities(t *testing.T) {
	description := Definition().Spec.Description
	for _, required := range []string{"tools[name](args)", `describeNamespace("cua")`, "cua.getState()", "documentation()", "observe after actions", `describeNamespace("agent")`, "agent.run({task,artifactRefs})", "Main has no agent namespace", "{text,artifactRefs}", "unawaited", "Approval"} {
		if !strings.Contains(description, required) {
			t.Fatalf("missing %q in %s", required, description)
		}
	}
}
