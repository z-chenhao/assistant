package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/config"
	"github.com/z-chenhao/assistant/internal/llm/memory"
	"github.com/z-chenhao/assistant/internal/skill"
	"github.com/z-chenhao/assistant/internal/tool"
	"github.com/z-chenhao/assistant/internal/tool/codemode"
)

// Exercise production composition rather than marking a test registry ourselves.
// New builtin registrations must inherit the single model-facing entry point.
func TestProductionToolSurfaceUsesOnlyCodeMode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("../../runtime/javascript/src/sidecar.mjs")
	if err != nil {
		t.Fatal(err)
	}
	for _, configured := range []bool{true, false} {
		name := "configured"
		if !configured {
			name = "missing-runtime-does-not-enable-native-fallback"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			cfg, err := config.Load("", func(key string) (string, bool) { return root, key == "STEWARD_DATA_ROOT" })
			if err != nil {
				t.Fatal(err)
			}
			if configured {
				cfg.JavaScript = &config.JavaScript{Command: node, Script: script}
			}
			registry, err := (&App{}).runtimeRegistry(context.Background(), cfg, "", memory.New(), nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, spec := range registry.Describe() {
				if spec.Name != codemode.ToolName && !spec.CodeModeOnly {
					t.Errorf("production registration permits native calls: %s", spec.Name)
				}
			}
			for role, specs := range map[string][]tool.Spec{"main": agent.MainTools(registry.Describe()), "worker": agent.WorkerTools(registry.Describe())} {
				// Treat every frozen tool as loaded to test the strongest checkpoint
				// forgery, not just the ordinary initial projection.
				loaded := make([]string, 0, len(specs))
				for _, spec := range specs {
					loaded = append(loaded, spec.Name)
				}
				declared := agent.ProviderTools(specs, loaded)
				if configured {
					if len(declared) != 1 || declared[0].Name != codemode.ToolName {
						t.Fatalf("%s declares native tools: %+v", role, declared)
					}
				} else if len(declared) != 0 {
					t.Fatalf("%s falls back to native tools without runtime: %+v", role, declared)
				}
			}
		})
	}
}

func TestCoreExecutionAndSkillDiscoveryAreAvailableWithoutOptionalConfig(t *testing.T) {
	root := t.TempDir()
	cfg, err := config.Load("", func(key string) (string, bool) { return root, key == "STEWARD_DATA_ROOT" })
	if err != nil {
		t.Fatal(err)
	}
	registry, err := (&App{}).runtimeRegistry(context.Background(), cfg, "", memory.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"builtin__read", "builtin__bash", "builtin__edit", "builtin__write", "builtin__skill_list", "builtin__skill_view", "builtin__image_ocr"} {
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
	if result := list.Execute(context.Background(), `{}`); result.Error != nil || !strings.Contains(result.Text, `"name":"intelligent_ui"`) {
		t.Fatal("builtin catalog not usable", result)
	}
	if _, err := os.Stat(cfg.Execution.Root); !os.IsNotExist(err) {
		t.Fatal("registry discovery created execution files")
	}
}

func TestRuntimePreparationRetriesAfterDependencyRepair(t *testing.T) {
	root := t.TempDir()
	cfg, err := config.Load("", func(key string) (string, bool) { return root, key == "STEWARD_DATA_ROOT" })
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
