package setup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-chenhao/collie/internal/computer/macosvm"
	"github.com/z-chenhao/collie/internal/computer/macosvm/fixture"
	"github.com/z-chenhao/collie/internal/config"
	"github.com/z-chenhao/collie/internal/interaction/web"
	"github.com/z-chenhao/collie/internal/tool/mcp"
	mcpfake "github.com/z-chenhao/collie/internal/tool/mcp/fake"
)

func TestSetupMCPPeer(t *testing.T) {
	for i, v := range os.Args {
		if v == "collie-setup-mcp" && i+1 < len(os.Args) {
			if mcpfake.Serve(os.Stdin, os.Stdout, os.Args[i+1]) != nil {
				os.Exit(2)
			}
			os.Exit(0)
		}
	}
}
func TestMCPProbeDiscoversWithoutInvokingEffects(t *testing.T) {
	m := fixtureManager(t)
	binary, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	cfg := []mcp.Config{{Name: "owned", Command: binary, Directory: root, Args: []string{"-test.run=^TestSetupMCPPeer$", "--", "collie-setup-mcp", root}, Tools: map[string]string{"read": "read_only", "send": "external_effect"}}}
	v := verify(t, m, "mcp", encode(cfg))
	if v.State != "ready" {
		t.Fatal(v)
	}
	if _, e := os.Stat(filepath.Join(root, "mcp-effects.jsonl")); !os.IsNotExist(e) {
		t.Fatal("probe called an effect", e)
	}
	left, _ := filepath.Glob(filepath.Join(root, ".collie-mcp-*"))
	if len(left) != 0 {
		t.Fatal(left)
	}
	cfg[0].Tools = map[string]string{"not_advertised": "read_only"}
	v0 := detail(t, m, "mcp")
	bad, e := m.Test(context.Background(), "mcp", v0.Revision, encode(cfg))
	if e != nil || bad.Verified {
		t.Fatal(bad, e)
	}
}
func TestComputerProbeChecksSealedBaseAndRuntimeWithoutBoot(t *testing.T) {
	m := fixtureManager(t)
	root := t.TempDir()
	peer, e := fixture.New(root)
	if e != nil {
		t.Fatal(e)
	}
	defer peer.Close()
	source := t.TempDir()
	for _, name := range []string{"disk.img", "nvram.bin"} {
		if e = os.WriteFile(filepath.Join(source, name), []byte("owned-base"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	os.WriteFile(filepath.Join(source, "config.json"), []byte(`{"macAddress":"02:00:00:00:00:01","machineIdentifier":"fixture-base","cpuCount":4}`), 0600)
	if e = macosvm.ProvisionBase(context.Background(), root, source, peer.Config.Compatibility); e != nil {
		t.Fatal(e)
	}
	t.Setenv("COLLIE_COMPUTER_FIXTURE_PASSWORD", "fixture-only")
	node, e := exec.LookPath("node")
	if e != nil {
		t.Fatal(e)
	}
	script, e := filepath.Abs("../../../runtime/javascript/src/sidecar.mjs")
	if e != nil {
		t.Fatal(e)
	}
	// A fresh source document omits the implicit local workspace override.
	os.WriteFile(m.path, []byte("version: 1\n"), 0600)
	text := encode(computerSettings{Computer: &peer.Config, JavaScript: &config.JavaScript{Command: node, Script: script}})
	v := verify(t, m, "computer", text)
	if e = m.Save(context.Background(), "computer", v.Revision, text); e != nil {
		t.Fatal(e)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "vms"))
	if len(entries) != 0 {
		t.Fatal("probe created a VM", entries)
	}
	if strings.Contains(v.Message, "fixture-only") {
		t.Fatal("credential exposed")
	}
	os.WriteFile(filepath.Join(root, "base", "disk.img"), []byte("corrupt"), 0600)
	v = detail(t, m, "computer")
	bad, e := m.Test(context.Background(), "computer", v.Revision, text)
	if e != nil || bad.Verified {
		t.Fatal(bad, e)
	}
	if e = m.Save(context.Background(), "computer", v.Revision, text); e != web.ErrCapabilityUnverified {
		t.Fatal(e)
	}
}

func TestBootstrapRetainsComputerDocumentShape(t *testing.T) {
	m := fixtureManager(t)
	peer, e := fixture.New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer peer.Close()
	node, e := exec.LookPath("node")
	if e != nil {
		t.Fatal(e)
	}
	script, e := filepath.Abs("../../../runtime/javascript/src/sidecar.mjs")
	if e != nil {
		t.Fatal(e)
	}
	m.active.Computer = &peer.Config
	m.active.Execution = nil
	m.active.JavaScript = &config.JavaScript{Command: node, Script: script}
	raw := m.bootstrap()
	decoded, e := config.Decode(raw, os.LookupEnv)
	if e != nil || decoded.Computer == nil || decoded.JavaScript == nil {
		t.Fatal(string(raw), e)
	}
	v := detail(t, m, "execution")
	if v.State != "managed" {
		t.Fatal(v)
	}
}
