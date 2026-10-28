package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/tool"
)

func TestContextRolesAndFrozenResources(t *testing.T) {
	specs := []tool.Spec{
		{Name: "builtin__workspace__read", PromptContext: []tool.PromptContext{{Kind: "project_instructions", Source: "workspace:AGENTS.md:hash", Text: "preserve user files"}}},
		{Name: "builtin__skill__list", PromptContext: []tool.PromptContext{{Kind: "skill_index", Source: "skills:version", Text: `[{"name":"research","version":"frozen"}]`}}},
		{Name: RunToolName},
	}
	root := InitialContext(ContextOptions{Position: "root", Tools: specs, Title: "request", Task: "Keep the exact constraints", ArtifactRefs: []string{"artifact:input"}})
	last := root[len(root)-1]
	if last.Source == nil || !last.Source.Protected || last.Source.Kind != "task" || !strings.Contains(messageText(last), "artifact:input") {
		t.Fatal("task and references are not one protected input", last)
	}
	root[0].Content[0].Text = "frozen parent safety"
	child := InitialContext(ContextOptions{Position: "child", Tools: specs, Task: "explicit subtask", ParentSystem: append(root, TextMessage("user", "PARENT_PRIVATE"))})
	raw, _ := json.Marshal(child)
	if !strings.Contains(string(raw), "frozen parent safety") || strings.Contains(string(raw), "PARENT_PRIVATE") || strings.Contains(string(raw), rootIdentity) {
		t.Fatal("child scope or frozen rules drift", string(raw))
	}
	for _, needle := range []string{"project_instructions", "workspace:AGENTS.md:hash", "skill_index", "frozen", "Delegation:"} {
		if !strings.Contains(string(raw), needle) {
			t.Fatal("missing scoped guidance", needle)
		}
	}
	main := InitialContext(ContextOptions{Position: "main", Tools: []tool.Spec{{Name: "builtin__memory__search"}}})
	b, _ := json.Marshal(main)
	if strings.Contains(string(b), "Delegation:") || strings.Contains(string(b), "Workspace:") {
		t.Fatal("guidance for absent capability")
	}
	if len(ProviderTools(specs)) != 3 || strings.Contains(ProviderTools(specs)[0].Description, "preserve user files") {
		t.Fatal("Host prompt metadata leaked into Tool declaration")
	}
}
