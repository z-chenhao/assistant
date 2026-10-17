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
	if !NamespaceMatches("builtin__text__echo", "text") || !NamespaceMatches("mcp__mail__read", "mcp__mail") || NamespaceMatches("mcp__mail__read", "agent") {
		t.Fatal("namespace mapping")
	}
}
