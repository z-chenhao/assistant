package app

import (
	"context"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/agent/sqlite"
	"github.com/z-chenhao/assistant/internal/config"
	"github.com/z-chenhao/assistant/internal/connectedapps"
	"github.com/z-chenhao/assistant/internal/llm/memory"
	"github.com/z-chenhao/assistant/internal/tool"
)

func TestConnectedAppsProductionCompositionAndRefresh(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := sqlite.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	a := &App{store: store, connectedApps: &connectedapps.Service{Store: store, Secrets: memory.New()}}
	cfg, err := config.Load("", func(key string) (string, bool) { return root, key == "STEWARD_DATA_ROOT" })
	if err != nil {
		t.Fatal(err)
	}
	registry, err := a.runtimeRegistry(ctx, cfg, "", memory.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := tool.FixtureRegistry().Resolve("mcp__fixture__echo")
	other.Spec.Name = "mcp__apps_personal__echo"
	if err = registry.ReplaceGroup("mcp__apps_personal__", []tool.Definition{other}); err != nil {
		t.Fatal(err)
	}
	a.core.Store(&runtimeCore{registry: registry})
	frozen := agent.MainTools(registry.Describe())
	state, _ := store.LoadApps(ctx)
	state.KeyRef = "env:COMPOSIO_API_KEY"
	state.Connections = append(state.Connections, connectedapps.Connection{ID: "test-binding", Name: "Gmail", Toolkit: "gmail", Alias: "work", AccountID: "ca_work", Status: "active"})
	if err = store.SaveApps(ctx, state, state.Revision); err != nil {
		t.Fatal(err)
	}
	if err = a.refreshConnectedApps(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Resolve(other.Spec.Name); !ok {
		t.Fatal("native app refresh removed a configured MCP server")
	}
	for _, specs := range [][]tool.Spec{agent.MainTools(registry.Describe()), agent.WorkerTools(registry.Describe())} {
		count := 0
		for _, spec := range specs {
			if strings.HasPrefix(spec.Name, "mcp__apps__b__") {
				count++
				if !spec.CodeModeOnly {
					t.Fatal("native model call enabled")
				}
			}
		}
		if count != 2 {
			t.Fatal("connected account not available to new work", count)
		}
	}
	selected, err := registry.Select(frozen)
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range selected.Describe() {
		if strings.HasPrefix(spec.Name, "mcp__apps__b__") {
			t.Fatal("old Main gained a new account")
		}
	}
}
