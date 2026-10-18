package skill

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishedMetadataRoundTripsThroughCatalog(t *testing.T) {
	for _, name := range []string{"verified-cli", "verified---cli", "---"} {
		for _, description := range []string{"Review code --- safely.", "---", "Quotes: \"safe\"; paths: C:\\work; # YAML: [x] {y}\nNext --- line.", "中文 — review --- safely."} {
			t.Run(name+"/"+description, func(t *testing.T) {
				c := Config{Root: t.TempDir(), Scope: "agent"}
				const procedure = "1. Read source.\n2. Verify --- boundaries.\n---\nRetain original evidence."
				request := ReviewRequest{Operation: "propose", Name: name, Description: description, Procedure: procedure, Reason: "success", SourceRefs: []string{"run:fixture"}}
				if !request.Valid() {
					t.Fatal("round-trip fixture is not accepted proposal metadata")
				}
				ctx := context.Background()
				for i := 0; i < 2; i++ {
					if err := Publish(ctx, c, "candidate", name, description, procedure); err != nil {
						t.Fatal(err)
					}
				}
				defs, err := Definitions(c)
				if err != nil {
					t.Fatal("published metadata prevents startup", err)
				}
				var summaries []Summary
				if err = json.Unmarshal([]byte(defs[0].Execute(ctx, `{}`).Text), &summaries); err != nil || len(summaries) != 1 || summaries[0].Name != name || summaries[0].Description != description {
					t.Fatal("metadata did not round-trip", summaries, err)
				}
				raw, _ := json.Marshal(map[string]string{"name": name, "version": summaries[0].Version})
				view := defs[1].Execute(ctx, string(raw))
				if view.Error != nil || !strings.HasSuffix(view.Text, procedure+"\n") {
					t.Fatal("body changed", view)
				}
				if err = Retire(c, "candidate", name, description, procedure); err != nil {
					t.Fatal(err)
				}
				defs, err = Definitions(c)
				if err != nil || defs[0].Execute(ctx, `{}`).Text != "[]" {
					t.Fatal("retired catalog unreadable", err)
				}
			})
		}
	}
}

func TestPublishRejectsUnreadableMetadataBeforeWriting(t *testing.T) {
	for _, tc := range []struct{ name, description, procedure string }{
		{"invalid_name", "Review code.", "1. Read source.\n2. Test it."},
		{"candidate", "", "1. Read source.\n2. Test it."},
		{"candidate", strings.Repeat("x", 1025), "1. Read source.\n2. Test it."},
		{"candidate", "Review code.", strings.Repeat("x", 65536)},
	} {
		c := catalog(t)
		before, err := os.ReadFile(filepath.Join(c.Root, "review", "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if err = Publish(context.Background(), c, "candidate", tc.name, tc.description, tc.procedure); err == nil {
			t.Fatal("unreadable Skill published", tc.name)
		}
		if _, err = Definitions(c); err != nil {
			t.Fatal("failed publication broke existing catalog", err)
		}
		entries, err := os.ReadDir(c.Root)
		if err != nil || len(entries) != 1 {
			t.Fatal("failed publication wrote a candidate", entries, err)
		}
		after, err := os.ReadFile(filepath.Join(c.Root, "review", "SKILL.md"))
		if err != nil || string(after) != string(before) {
			t.Fatal("existing Skill changed", err)
		}
	}
}
