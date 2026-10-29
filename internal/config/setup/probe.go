package setup

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/z-chenhao/collie/internal/computer/environment"
	"github.com/z-chenhao/collie/internal/computer/macosvm"
	"github.com/z-chenhao/collie/internal/config"
	"github.com/z-chenhao/collie/internal/memory"
	"github.com/z-chenhao/collie/internal/recall/embedding"
	"github.com/z-chenhao/collie/internal/skill"
	"github.com/z-chenhao/collie/internal/tool/codemode"
	"github.com/z-chenhao/collie/internal/tool/mcp"
)

func (m *Manager) probe(ctx context.Context, c config.Config, id string) (string, string) {
	if id == "execution" && c.Computer != nil {
		return "unavailable", "File tools use Computer. Configure and test that backend instead."
	}
	if resource(c, id) == nil {
		return "ready", "This candidate disables the capability. Save and restart to apply it; existing work keeps its original capabilities."
	}
	switch id {
	case "memory":
		if _, e := (memory.Vault{Secrets: m.secrets, Ref: c.Memory.EncryptionKeyRef}).Digest(ctx, "Collie memory connection test"); e != nil {
			return "unavailable", "Memory key unavailable. Provide the referenced 32-byte key in SecretStore or the process environment."
		}
		return "ready", "The referenced 32-byte encryption key is available. No memory was read or written."
	case "embedding":
		v, e := (embedding.Client{Config: *c.Embedding, Secrets: m.secrets}).Embed(ctx, "Collie embedding connection test")
		if e != nil {
			return "unavailable", "Embedding connection failed. Check the endpoint, model, dimensions and credential reference; redirects and invalid vectors are rejected."
		}
		return "ready", fmt.Sprintf("Embedding endpoint returned a valid %d-dimensional vector for a fixed diagnostic sentence.", len(v))
	case "skills":
		defs, e := skill.Definitions(*c.Skills)
		if e != nil {
			return "unavailable", "Skill catalog unavailable. Prepare a readable catalog with valid SKILL.md files and bounded resources at the configured root."
		}
		return "ready", fmt.Sprintf("The actual Skill catalog and its resources passed validation (%d discovery/read tools).", len(defs))
	case "mcp":
		total := 0
		for _, server := range c.MCP {
			defs, e := mcp.Definitions(ctx, server)
			if e != nil {
				return "unavailable", "MCP discovery failed. Check absolute executable/private directory paths, the kernel sandbox, protocol and allowlisted tool names."
			}
			total += len(defs)
		}
		return "ready", fmt.Sprintf("%d isolated provider(s) advertised %d allowed tools. No advertised tool was invoked.", len(c.MCP), total)
	case "execution":
		r := (environment.Local{Config: c.ExecutionConfig()}).Execute(ctx, "builtin__bash", `{"command":"printf collie-connection-test","timeout":5}`)
		if r.Error != nil {
			return "unavailable", "Workspace diagnostic failed. Check private directory permissions, Python 3, bash, kernel sandbox permissions and read-only toolchain paths."
		}
		var result struct {
			ExitCode int    `json:"exit_code"`
			Text     string `json:"output"`
		}
		if json.Unmarshal([]byte(r.ModelText()), &result) != nil || result.ExitCode != 0 || result.Text != "collie-connection-test" {
			return "unavailable", "The isolated diagnostic command did not complete successfully."
		}
		return "ready", "A fixed diagnostic command completed in the configured kernel sandbox. No user command or network access was used."
	case "computer":
		if c.JavaScript == nil {
			return "unavailable", "Computer requires the JavaScript runtime configuration."
		}
		if e := (&codemode.Sidecar{Command: c.JavaScript.Command, Script: c.JavaScript.Script}).Check(ctx); e != nil {
			return "unavailable", "JavaScript runtime unavailable. Check the absolute Node/sidecar paths and install the pinned runtime dependencies."
		}
		p, e := macosvm.New(*c.Computer, macosvm.SSHGuest{Config: c.Computer.Guest, Secrets: m.secrets})
		if e != nil {
			return "unavailable", "Computer store unavailable. Provision the private Lume store and sealed base before testing."
		}
		defer p.Close()
		if e = p.Probe(ctx); e != nil {
			return "unavailable", "Computer connection failed. Check the Lume endpoint/version, sealed base compatibility/integrity and Guest credential reference. The diagnostic does not boot or modify a VM."
		}
		return "ready", "Lume version, sealed base and credential availability verified. Guest runtime identity is checked when a Run first opens its Computer."
	}
	return "unavailable", "Capability not available."
}
