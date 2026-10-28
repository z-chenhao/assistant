package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/prompt"
	"github.com/z-chenhao/erii/internal/tool"
)

// These hashes guard the complete projection, including whitespace, section
// order, frozen resources and explicit Artifact inputs. Update them only for
// intentional prompt or model-visible protocol changes.
func TestContextPromptBytes(t *testing.T) {
	t.Chdir(t.TempDir())
	specs := []tool.Spec{
		{Name: "builtin__workspace_read", PromptContext: []tool.PromptContext{{Kind: "project_instructions", Source: "workspace:AGENTS.md:frozen", Text: "Keep project rules."}}},
		{Name: "builtin__codemode_execute"},
		{Name: RunToolName},
		{Name: "builtin__artifact_manage"},
		{Name: "builtin__memory_search"},
		{Name: "builtin__skill_view"},
		{Name: "builtin__skill_list", PromptContext: []tool.PromptContext{{Kind: "skill_index", Source: "skills:frozen", Text: "research"}}},
		{Name: "builtin__computer_observe", Internal: true},
	}
	for _, tc := range []struct{ position, hash string }{{"main", "b251eae3d876e715a9ec6e78ba41e994772123d3972d5268edde320e0b53cf9f"}, {"root", "99b25c813061af61fe3fc16e4808988784b67a794726c76bd650dd03691f27ca"}, {"child", "20fb3a61f78f7648f4fb6f9a569add725b734cf4bbede268e0ad8250884c4430"}} {
		t.Run(tc.position, func(t *testing.T) {
			messages, err := ProviderMessages(InitialContext(ContextOptions{Position: tc.position, Tools: specs, Title: "Review", Task: "Keep constraints", ArtifactRefs: []string{"artifact:input"}}))
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(messages)
			got := fmt.Sprintf("%x", sha256.Sum256(raw))
			if got != tc.hash {
				t.Fatalf("model message bytes changed: %s", got)
			}
		})
	}
}

func TestContextRolesAndFrozenResources(t *testing.T) {
	specs := []tool.Spec{
		{Name: "builtin__workspace_read", PromptContext: []tool.PromptContext{{Kind: "project_instructions", Source: "workspace:AGENTS.md:hash", Text: "preserve user files"}}},
		{Name: "builtin__skill_list", PromptContext: []tool.PromptContext{{Kind: "skill_index", Source: "skills:version", Text: `[{"name":"research","version":"frozen"}]`}}},
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
	if !strings.Contains(string(raw), "frozen parent safety") || strings.Contains(string(raw), "PARENT_PRIVATE") || strings.Contains(string(raw), prompt.Identity()) {
		t.Fatal("child scope or frozen rules drift", string(raw))
	}
	for _, needle := range []string{"project_instructions", "workspace:AGENTS.md:hash", "skill_index", "frozen", "Delegation:"} {
		if !strings.Contains(string(raw), needle) {
			t.Fatal("missing scoped guidance", needle)
		}
	}
	main := InitialContext(ContextOptions{Position: "main", Tools: []tool.Spec{{Name: "builtin__memory_search"}}})
	b, _ := json.Marshal(main)
	if strings.Contains(string(b), "Delegation:") || strings.Contains(string(b), "Workspace:") {
		t.Fatal("guidance for absent capability")
	}
	if len(ProviderTools(specs)) != 3 || strings.Contains(ProviderTools(specs)[0].Description, "preserve user files") {
		t.Fatal("Host prompt metadata leaked into Tool declaration")
	}
}
