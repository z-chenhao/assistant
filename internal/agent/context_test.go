package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/prompt"
	"github.com/z-chenhao/assistant/internal/tool"
)

// These hashes guard the complete projection, including whitespace, section
// order, frozen resources and explicit Artifact inputs. Update them only for
// intentional prompt or model-visible protocol changes.
func TestContextPromptBytes(t *testing.T) {
	t.Chdir(t.TempDir())
	specs := []tool.Spec{
		{Name: "builtin__read", PromptContext: []tool.PromptContext{{Kind: "project_instructions", Source: "workspace:AGENTS.md:frozen", Text: "Keep project rules."}}},
		{Name: "builtin__codemode_execute"},
		{Name: RunToolName},
		{Name: "builtin__artifact_manage"},
		{Name: "builtin__memory_search"},
		{Name: "builtin__skill_view"},
		{Name: "builtin__skill_list", PromptContext: []tool.PromptContext{{Kind: "skill_index", Source: "skills:frozen", Text: `[{"name":"research","description":"Find and verify sources.","scope":"agent","version":"frozen"}]`}}},
		{Name: "builtin__computer_observe", Internal: true},
		{Name: "mcp__calendar__lookup", Provider: "mcp:calendar", NamespaceDescription: "Find calendar events", Exposure: tool.CodeMode, CodeModeOnly: true},
	}
	for _, tc := range []struct{ position, hash string }{{"main", "0497f281884ae2a016b9c815cf6287e1909d17f23f1c2340cbb4140e5ca7fe2a"}, {"root", "b54fb61aae5c0218e08fb34800df93717a7882c4c74c5ca68d27d1279376cdad"}, {"child", "82b6bf0711efec7748cf715ae1f82df5b77413ec4d04bdfca3f1367dbf823bc6"}} {
		t.Run(tc.position, func(t *testing.T) {
			messages, err := ProviderMessages(InitialContext(ContextOptions{Position: tc.position, Tools: specs, Title: "Review", Task: "Keep constraints", ArtifactRefs: []string{"artifact:input"}}), false)
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
		{Name: "builtin__read", PromptContext: []tool.PromptContext{{Kind: "project_instructions", Source: "workspace:AGENTS.md:hash", Text: "preserve user files"}}},
		{Name: "builtin__skill_list", PromptContext: []tool.PromptContext{{Kind: "skill_index", Source: "skills:version", Text: `[{"name":"research","description":"Use frozen research guidance.","version":"frozen"}]`}}},
		{Name: RunToolName},
	}
	root := InitialContext(ContextOptions{Position: "root", Tools: specs, Title: "request", Task: "Keep the exact constraints", ArtifactRefs: []string{"artifact:input"}})
	last := root[len(root)-1]
	if last.Source == nil || !last.Source.Protected || last.Source.Kind != "task" || !strings.Contains(messageText(last), "artifact:input") {
		t.Fatal("task and references are not one protected input", last)
	}
	for i := range root[0].SystemSections {
		section := &root[0].SystemSections[i]
		switch section.Kind {
		case "safety":
			section.Text = "frozen parent safety"
		case "worker_role":
			section.ChildText = "frozen parent Worker role"
			section.Text = prompt.Identity() + "\n\n" + section.ChildText
		case "execution":
			section.ChildText = "<execution_rules>\nfrozen parent execution\n</execution_rules>"
			section.Text = "<execution_rules>\nfrozen parent execution\n" + prompt.Progress() + "\n</execution_rules>"
		}
	}
	texts := []string{}
	for _, section := range root[0].SystemSections {
		texts = append(texts, section.Text)
	}
	root[0].Content[0].Text = strings.Join(texts, "\n\n")
	frozen, err := json.Marshal(root)
	if err != nil || json.Unmarshal(frozen, &root) != nil {
		t.Fatal("frozen sections did not round-trip", err)
	}
	child := InitialContext(ContextOptions{Position: "child", Tools: specs, Task: "explicit subtask", ParentSystem: append(root, TextMessage("user", "PARENT_PRIVATE"))})
	raw, _ := json.Marshal(child)
	if !strings.Contains(string(raw), "frozen parent safety") || strings.Contains(string(raw), "PARENT_PRIVATE") || strings.Contains(string(raw), prompt.Identity()) || strings.Contains(string(raw), prompt.Progress()) {
		t.Fatal("child scope or frozen rules drift", string(raw))
	}
	for _, needle := range []string{"frozen parent safety", "frozen parent Worker role", "frozen parent execution", "Delegation:"} {
		if !strings.Contains(messageText(child[0]), needle) {
			t.Fatal("missing scoped guidance", needle)
		}
	}
	main := InitialContext(ContextOptions{Position: "main", Tools: []tool.Spec{{Name: "builtin__memory_search"}}})
	b, _ := json.Marshal(main)
	if strings.Contains(string(b), "Delegation:") || strings.Contains(string(b), "Execution environment:") {
		t.Fatal("guidance for absent capability")
	}
	if len(ProviderTools(specs)) != 3 || strings.Contains(ProviderTools(specs)[0].Description, "preserve user files") {
		t.Fatal("Host prompt metadata leaked into Tool declaration")
	}
}
