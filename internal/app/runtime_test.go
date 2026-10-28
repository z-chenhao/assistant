package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/config"
	"github.com/z-chenhao/erii/internal/llm/memory"
	"github.com/z-chenhao/erii/internal/skill"
)

func TestCoreExecutionAndSkillDiscoveryAreAvailableWithoutOptionalConfig(t *testing.T) {
	root := t.TempDir()
	cfg, err := config.Load("", func(key string) (string, bool) { return root, key == "ERII_DATA_ROOT" })
	if err != nil {
		t.Fatal(err)
	}
	registry, err := (&App{}).runtimeRegistry(context.Background(), cfg, "", memory.New())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"builtin__read", "builtin__bash", "builtin__edit", "builtin__write", "builtin__skill_list", "builtin__skill_view"} {
		if _, ok := registry.Resolve(name); !ok {
			t.Fatal("missing core tool", name)
		}
	}
	for _, spec := range registry.Describe() {
		if strings.HasPrefix(spec.Name, "builtin__workspace_") {
			t.Fatal("legacy tool remains", spec.Name)
		}
	}
	list, _ := registry.Resolve("builtin__skill_list")
	if result := list.Execute(context.Background(), `{}`); result.Error != nil || result.Text != "[]" {
		t.Fatal("empty catalog not usable", result)
	}
	if _, err := os.Stat(cfg.Execution.Root); !os.IsNotExist(err) {
		t.Fatal("registry discovery created execution files")
	}
}

func TestRuntimePreparationRetriesAfterDependencyRepair(t *testing.T) {
	root := t.TempDir()
	cfg, err := config.Load("", func(key string) (string, bool) { return root, key == "ERII_DATA_ROOT" })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Skills = &skill.Config{Root: filepath.Join(root, "skills"), Scope: "agent"}
	app, err := open(context.Background(), cfg, "", memory.New(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	original := app.core.Load()
	if err := app.prepareCore(context.Background()); err == nil {
		t.Fatal("unavailable catalog admitted")
	}
	if app.core.Load() != original {
		t.Fatal("partial runtime published after preparation failure")
	}
	if err := os.Mkdir(cfg.Skills.Root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := app.prepareCore(context.Background()); err != nil {
		t.Fatal("first preparation failure cached forever", err)
	}
	prepared := app.core.Load()
	if prepared == original || prepared.registry == nil {
		t.Fatal("repaired runtime not published")
	}
	if err := app.prepareCore(context.Background()); err != nil || app.core.Load() != prepared {
		t.Fatal("successful runtime reconstructed or drifted", err)
	}
}
