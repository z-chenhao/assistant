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
	}
	for _, tc := range []struct{ position, hash string }{{"main", "0497f281884ae2a016b9c815cf6287e1909d17f23f1c2340cbb4140e5ca7fe2a"}, {"root", "9d60c7e2985f3758e08ebc9a456e98ec7b2cb95f32bc373ec9eb115772e319a9"}, {"child", "9b4105495a6c01816804bb527b4f1a4d41f22d9dee22fa79671d2a2bc7913956"}} {
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
		if root[0].SystemSections[i].Kind == "safety" {
			root[0].SystemSections[i].Text = "frozen parent safety"
		}
	}
	texts := []string{}
	for _, section := range root[0].SystemSections {
		texts = append(texts, section.Text)
	}
	root[0].Content[0].Text = strings.Join(texts, "\n\n")
	child := InitialContext(ContextOptions{Position: "child", Tools: specs, Task: "explicit subtask", ParentSystem: append(root, TextMessage("user", "PARENT_PRIVATE"))})
	raw, _ := json.Marshal(child)
	if !strings.Contains(string(raw), "frozen parent safety") || strings.Contains(string(raw), "PARENT_PRIVATE") || strings.Contains(string(raw), prompt.Identity()) || strings.Contains(string(raw), prompt.Progress()) {
		t.Fatal("child scope or frozen rules drift", string(raw))
	}
	for _, needle := range []string{"frozen parent safety", "Delegation:"} {
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
