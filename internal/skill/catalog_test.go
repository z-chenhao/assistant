package skill

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/tool"
)

func catalog(t *testing.T) Config {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "review", "references"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "review", "SKILL.md"), []byte("---\nname: review\ndescription: Review code with file evidence.\n---\nBODY_ONLY_MARKER\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "review", "references", "rules.md"), []byte("reference"), 0600); err != nil {
		t.Fatal(err)
	}
	return Config{Root: root, Scope: "agent"}
}
func TestProgressiveCatalogScopeVersionsAndNoManagement(t *testing.T) {
	c := catalog(t)
	defs, err := Definitions(c)
	if err != nil {
		t.Fatal(err)
	}
	r, err := tool.NewRegistry(defs)
	if err != nil {
		t.Fatal(err)
	}
	list, _ := r.Resolve("builtin__skill_list")
	view, _ := r.Resolve("builtin__skill_view")
	if _, ok := r.Resolve("builtin__skill_manage"); ok {
		t.Fatal("management exposed")
	}
	if strings.Contains(list.Spec.Description, "BODY_ONLY_MARKER") {
		t.Fatal("full body in startup summary")
	}
	var summaries []Summary
	listing := list.Execute(context.Background(), `{}`).Text
	if json.Unmarshal([]byte(listing), &summaries) != nil || len(summaries) != 1 || summaries[0].Name != "review" || strings.Contains(listing, `"version"`) || strings.Contains(listing, `"scope"`) {
		t.Fatal(summaries)
	}
	var frozenEntries []Summary
	if json.Unmarshal([]byte(list.Spec.PromptContext[0].Text), &frozenEntries) != nil || len(frozenEntries) != 1 || frozenEntries[0].Scope != "agent" || frozenEntries[0].Version == "" {
		t.Fatal("Host lost frozen resource identity", list.Spec.PromptContext)
	}
	invoke := func(path string) tool.Result {
		raw, _ := json.Marshal(map[string]string{"name": "review", "path": path})
		return view.Execute(context.Background(), string(raw))
	}
	if got := invoke(""); got.Error != nil || !strings.Contains(got.Text, "BODY_ONLY_MARKER") {
		t.Fatal(got)
	}
	if got := invoke("references/rules.md"); got.Text != "reference" {
		t.Fatal(got)
	}
	for _, p := range []string{"../SKILL.md", "/etc/passwd", "missing.md"} {
		if got := invoke(p); got.Error == nil {
			t.Fatal(got)
		}
	}
	if got := view.Execute(context.Background(), `{"name":"review","version":"injected"}`); got.Error == nil || got.Error.Code != "INVALID_ARGUMENT" {
		t.Fatal(got)
	}
	frozen := r.Describe()
	if err := os.WriteFile(filepath.Join(c.Root, "review", "references", "rules.md"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := invoke("references/rules.md"); got.Error == nil || got.Error.Code != "NON_DETERMINISTIC_REPLAY" {
		t.Fatal(got)
	}
	changed, _ := Definitions(c)
	current, _ := tool.NewRegistry(changed)
	if _, err := current.Select(frozen); err == nil {
		t.Fatal("resumed old catalog with new content")
	}
	if SetHash(frozen) == SetHash(current.Describe()) {
		t.Fatal("hash ignored reference")
	}
	c.Scope = "workspace"
	scoped, _ := Definitions(c)
	scopedRegistry, _ := tool.NewRegistry(scoped)
	if SetHash(current.Describe()) == SetHash(scopedRegistry.Describe()) {
		t.Fatal("hash ignored scope")
	}
}

func TestSkillSearchRanksBoundsAndFreezesResults(t *testing.T) {
	c := catalog(t)
	fillCatalog(t, c, 12)
	if err := Publish(context.Background(), c, "plot", "plotter", "绘制图表并比较趋势。", "1. Inspect data.\n2. Plot verified values."); err != nil {
		t.Fatal(err)
	}
	defs, err := Definitions(c)
	if err != nil {
		t.Fatal(err)
	}
	defs, err = IncludeIntelligentUI(defs)
	if err != nil {
		t.Fatal(err)
	}
	search := func(args string) []map[string]string {
		t.Helper()
		result := defs[0].Execute(context.Background(), args)
		var entries []map[string]string
		if result.Error != nil || json.Unmarshal([]byte(result.Text), &entries) != nil {
			t.Fatal(result)
		}
		for _, entry := range entries {
			if len(entry) != 2 || entry["name"] == "" || entry["description"] == "" {
				t.Fatal("search exposed internal metadata or body", entries)
			}
		}
		return entries
	}
	if entries := search(`{}`); len(entries) != 8 || entries[0]["name"] != "fixture-000" {
		t.Fatal("unbounded or unordered browsing", entries)
	}
	if entries := search(`{"query":"Fixture method","limit":2}`); len(entries) != 2 {
		t.Fatal("limit ignored", entries)
	}
	for query, want := range map[string]string{"INTELLIGENT_UI": "intelligent_ui", "Review code": "review", "图表": "plotter"} {
		args, _ := json.Marshal(map[string]any{"query": query, "limit": 1})
		if entries := search(string(args)); len(entries) != 1 || entries[0]["name"] != want {
			t.Fatal("name or purpose search failed", query, entries)
		}
	}
	if entries := search(`{"query":"nonexistentmethod"}`); len(entries) != 0 {
		t.Fatal("unrelated search returned methods", entries)
	}
	for _, args := range []string{`{"limit":0}`, `{"limit":129}`, `{"limit":1.5}`, `{"query":12}`, `{"version":"injected"}`} {
		if got := defs[0].Execute(context.Background(), args); got.Error == nil || got.Error.Code != "INVALID_ARGUMENT" {
			t.Fatal("invalid search accepted", args, got)
		}
	}
	if err := Publish(context.Background(), c, "news", "fresh-news", "New method.", "1. Inspect sources."); err != nil {
		t.Fatal(err)
	}
	if entries := search(`{"query":"fresh-news"}`); len(entries) != 0 {
		t.Fatal("new publication entered an already frozen catalog", entries)
	}
}

func TestSkillSearchFindsPunctuationOnlyName(t *testing.T) {
	c := catalog(t)
	if err := Publish(context.Background(), c, "punctuation", "---", "Retained method.", "1. Inspect sources."); err != nil {
		t.Fatal(err)
	}
	defs, err := Definitions(c)
	if err != nil {
		t.Fatal(err)
	}
	var entries []Summary
	result := defs[0].Execute(context.Background(), `{"query":"---","limit":1}`)
	if result.Error != nil || json.Unmarshal([]byte(result.Text), &entries) != nil || len(entries) != 1 || entries[0].Name != "---" {
		t.Fatal("valid catalog name could not be found", result)
	}
}
func TestSkillLinksAndMalformedFrontMatterRejected(t *testing.T) {
	for _, variant := range []string{"link", "name", "description"} {
		t.Run(variant, func(t *testing.T) {
			c := catalog(t)
			switch variant {
			case "link":
				if err := os.Symlink("/etc/passwd", filepath.Join(c.Root, "review", "references", "outside")); err != nil {
					t.Fatal(err)
				}
			case "name":
				if err := os.WriteFile(filepath.Join(c.Root, "review", "SKILL.md"), []byte("---\nname: other\ndescription: x\n---\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "description":
				if err := os.WriteFile(filepath.Join(c.Root, "review", "SKILL.md"), []byte("---\nname: review\n---\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := Definitions(c); err == nil {
				t.Fatal("invalid catalog accepted")
			}
		})
	}
}

func TestCatalogFrontmatterUsesDelimiterLines(t *testing.T) {
	for _, tc := range []struct {
		name, text, description string
		valid                   bool
	}{
		{"quoted", "---\nname: review\ndescription: \"Review --- safely.\"\n---\nBody --- text.", "Review --- safely.", true},
		{"crlf", "---\r\nname: review\r\ndescription: Review --- safely.\r\n---\r\nBody.", "Review --- safely.", true},
		{"block", "---\nname: review\ndescription: |\n  Review\n  ---\n  safely.\n---\nBody.", "Review\n---\nsafely.\n", true},
		{"inline-opening", "---name: review\ndescription: Review.\n---\nBody.", "", false},
		{"inline-closing", "---\nname: review\ndescription: Review --- safely.\nBody.", "", false},
		{"missing-closing", "---\nname: review\ndescription: Review.\n---invalid\nBody.", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := catalog(t)
			if err := os.WriteFile(filepath.Join(c.Root, "review", "SKILL.md"), []byte(tc.text), 0600); err != nil {
				t.Fatal(err)
			}
			defs, err := Definitions(c)
			if !tc.valid {
				if err == nil {
					t.Fatal("malformed frontmatter accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var summaries []Summary
			if err = json.Unmarshal([]byte(defs[0].Execute(context.Background(), `{}`).Text), &summaries); err != nil || len(summaries) != 1 || summaries[0].Description != tc.description {
				t.Fatal(summaries, err)
			}
		})
	}
}
