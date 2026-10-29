// Package setup owns user configuration editing and explicit readiness probes.
// It is independent of Agent execution, model capability discovery and Run state.
package setup

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/z-chenhao/collie/internal/computer/macosvm"
	"github.com/z-chenhao/collie/internal/config"
	"github.com/z-chenhao/collie/internal/interaction/web"
	"go.yaml.in/yaml/v3"
)

type computerSettings struct {
	Computer   *macosvm.Config    `yaml:"computer"`
	JavaScript *config.JavaScript `yaml:"javascript"`
}

var kinds = []string{"memory", "embedding", "mcp", "skills", "execution", "computer"}

func resource(c config.Config, id string) any {
	switch id {
	case "memory":
		if c.Memory != nil {
			return c.Memory
		}
	case "embedding":
		if c.Embedding != nil {
			return c.Embedding
		}
	case "mcp":
		if len(c.MCP) > 0 {
			return c.MCP
		}
	case "skills":
		if c.Skills != nil {
			return c.Skills
		}
	case "execution":
		if c.Computer == nil {
			return c.ExecutionConfig()
		}
	case "computer":
		if c.Computer != nil {
			return computerSettings{c.Computer, c.JavaScript}
		}
	}
	return nil
}
func encode(v any) string {
	if v == nil {
		return ""
	}
	raw, _ := yaml.Marshal(v)
	return string(raw)
}
func description(id, root string) web.Capability {
	v := web.Capability{ID: id, State: "not_configured"}
	parent := filepath.Dir(root)
	switch id {
	case "memory":
		v.Title = "Memory"
		v.Description = "Remember preferences and corrections across conversations."
		v.Help = "Use an existing SecretStore reference to the same 32-byte encryption key. Keep this key for existing memories and backups. Secret values never belong in this editor."
		v.Example = "encryption_key_ref: env:COLLIE_MEMORY_KEY\n"
	case "embedding":
		v.Title = "Memory search"
		v.Description = "Search memories and conversation with an explicit embedding model."
		v.Help = "Serve your chosen model separately. Encrypted memory requires a literal loopback endpoint. The test embeds a fixed diagnostic sentence, never your conversation. Changing an existing endpoint, model or dimensions requires an offline index rebuild; a successful connection test does not migrate stored vectors."
		v.Example = "provider: openai_compatible\nendpoint: http://127.0.0.1:11434/v1\nmodel: nomic-embed-text\ndimensions: 768\n"
	case "mcp":
		v.Title = "MCP tools"
		v.Description = "Connect allowlisted tools from isolated local stdio providers."
		v.Help = "Use absolute executable and private state paths. Testing starts the configured providers in the existing kernel sandbox and checks tool discovery; it never invokes an advertised tool. Credentials in command arguments are not supported."
		v.Example = fmt.Sprintf("- name: local_catalog\n  command: /absolute/path/to/mcp-server\n  args: []\n  directory: %s\n  tools: {lookup: read_only}\n", filepath.Join(parent, "collie-mcp"))
	case "skills":
		v.Title = "Skills"
		v.Description = "Discover curated, versioned methods and their resources."
		v.Help = "Prepare the catalog outside application data and execution workspaces. Testing validates the actual catalog and its bounded resources. An empty valid catalog is allowed."
		v.Example = fmt.Sprintf("root: %s\nscope: agent\n", filepath.Join(parent, "collie-skills"))
	case "execution":
		v.Title = "Files and commands"
		v.Description = "Read, edit, write and run commands in a private isolated workspace."
		v.Help = "Use a separate private directory and optional read-only toolchains. Testing creates the workspace if needed and runs only a fixed diagnostic command in the kernel sandbox. Disable Computer first to select this backend. An empty setting restores the default local workspace."
		v.Example = fmt.Sprintf("root: %s\ntoolchain_roots: []\n", root+".computer")
	case "computer":
		v.Title = "Computer"
		v.Description = "Use an explicitly provisioned macOS Guest for desktop and file tools."
		v.Help = "Provision the Lume store and sealed base first; this page does not download or boot a VM. Testing checks the Lume version, sealed base and credential availability. Guest runtime checks still run on first use. Clear the local execution override before enabling Computer."
		v.Example = "root: /absolute/private/computer-store\nendpoint: http://127.0.0.1:7777\ncompatibility:\n  provider_version: '0.6.1'\n  guest_build: YOUR_BUILD\n  driver_version: YOUR_DRIVER_VERSION\n  driver_sha256: YOUR_DRIVER_SHA256\n  driver_team: YOUR_DRIVER_TEAM\n  driver_bundle: com.trycua.driver\n  architecture: arm64\nguest:\n  user: lume\n  port: 22\n  password_ref: env:COLLIE_GUEST_PASSWORD\n  host_key_sha256: SHA256:YOUR_PINNED_HOST_KEY\nmanifest:\n  version: 1\n"
	}
	if id == "computer" {
		raw := v.Example
		lines := strings.Split(strings.TrimSuffix(raw, "\n"), "\n")
		for i := range lines {
			lines[i] = "  " + lines[i]
		}
		v.Example = "computer:\n" + strings.Join(lines, "\n") + "\njavascript:\n  command: /absolute/path/to/node\n  script: /absolute/path/to/runtime/javascript/src/sidecar.mjs\n"
		v.Help += " Include computer and javascript sections in this editor."
	}
	return v
}
