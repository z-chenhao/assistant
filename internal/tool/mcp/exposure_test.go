package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/tool"
	mcpfake "github.com/z-chenhao/assistant/internal/tool/mcp/fake"
	"go.yaml.in/yaml/v3"
)

func TestMCPConfigurationHasOneInvocationPath(t *testing.T) {
	c := config(t)
	for _, raw := range []string{"exposure: codemode\n", "exposure: direct\n", "exposure: deferred\n", "exposure: hidden\n", "tool_exposure: {read: direct}\n"} {
		decoder := yaml.NewDecoder(strings.NewReader(raw))
		decoder.KnownFields(true)
		if decoder.Decode(&c) == nil {
			t.Fatalf("obsolete exposure setting accepted: %s", raw)
		}
	}
	defs, err := Definitions(context.Background(), config(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range defs {
		if d.Spec.Exposure != tool.CodeMode || !d.Spec.CodeModeOnly || !d.Spec.ScriptCallable() || d.Spec.ModelVisible(map[string]bool{d.Spec.Name: true}) {
			t.Fatal("MCP bypasses Code Mode", d.Spec)
		}
	}
}

func TestMCPDiscovers500ToolsWithoutChangingExecutionIdentity(t *testing.T) {
	c := config(t)
	c.Tools = map[string]string{}
	catalog := []map[string]any{}
	for i := 0; i < 500; i++ {
		name := fmt.Sprintf("lookup_%03d", i)
		c.Tools[name] = "read_only"
		catalog = append(catalog, map[string]any{"name": name, "description": "Lookup a catalog record", "inputSchema": json.RawMessage(mcpfake.Schema)})
	}
	raw, _ := json.Marshal(map[string]any{"tools": catalog})
	if err := os.WriteFile(filepath.Join(c.Directory, "mcp-tools.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	defs, err := Definitions(context.Background(), c)
	if err != nil || len(defs) != 500 {
		t.Fatal("large MCP catalog rejected", len(defs), err)
	}
	legacy := defs[0].Spec
	legacy.Exposure = ""
	legacy.CodeModeOnly = false
	registry, err := tool.NewRegistry(defs)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := registry.Select([]tool.Spec{legacy})
	if err != nil || selected.Describe()[0].Exposure != "" {
		t.Fatal("historical frozen snapshot changed", err)
	}
	c.Description = "New catalog summary"
	direct, err := Definitions(context.Background(), c)
	if err != nil || direct[0].Spec.ResourceHash != defs[0].Spec.ResourceHash || !direct[0].Spec.CodeModeOnly || direct[0].Spec.Exposure != tool.CodeMode {
		t.Fatal("presentation rebound execution", err)
	}
}
