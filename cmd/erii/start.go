package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/z-chenhao/erii/internal/config"
)

// A source build and an installed bundle share the same relative layout. Use
// the executable, never cwd: users can launch Erii from any working directory.
func runtimeHome() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", errors.New("locate Erii installation")
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", errors.New("resolve Erii installation")
	}
	return filepath.Dir(filepath.Dir(path)), nil
}

func configureRuntime(cfg *config.Config, home string) {
	if cfg.LLM.ProviderBridgeCommand == "" {
		cfg.LLM = config.LLM{ProviderBridgeCommand: "node", ProviderBridgeScript: filepath.Join(home, "provider-bridge", "bridge.mjs"), ProviderBridgeVersion: "pi-ai-1.3.0-v1"}
	}
	if cfg.JavaScript == nil {
		cfg.JavaScript = &config.JavaScript{Command: "node", Script: filepath.Join(home, "runtime", "javascript", "src", "sidecar.mjs")}
	}
}

// Doctor is read-only: it does not open databases, contact providers, retrieve
// credentials, or select a model. An unavailable setup remains setup_required.
func doctor(ctx context.Context, cfg config.Config, out io.Writer) error {
	failed := false
	check := func(name string, err error) {
		if err != nil {
			failed = true
			fmt.Fprintln(out, "FAIL", name)
		} else {
			fmt.Fprintln(out, "OK  ", name)
		}
	}
	type runtimeCheck struct{ name, command, script, dependency string }
	var runtimes []runtimeCheck
	if cfg.LLM.ProviderBridgeCommand != "" {
		runtimes = append(runtimes, runtimeCheck{"model bridge", cfg.LLM.ProviderBridgeCommand, cfg.LLM.ProviderBridgeScript, "node_modules/@earendil-works/pi-ai/package.json"})
	} else {
		fmt.Fprintln(out, "SKIP model bridge: not configured; use default erii startup or add llm settings")
	}
	if cfg.JavaScript != nil {
		runtimes = append(runtimes, runtimeCheck{"JavaScript runtime", cfg.JavaScript.Command, cfg.JavaScript.Script, "../node_modules/quickjs-wasi/package.json"})
	}
	for _, runtime := range runtimes {
		value, err := exec.CommandContext(ctx, runtime.command, "--version").Output()
		if err == nil && strings.TrimSpace(string(value)) != "v24.21.0" {
			err = errors.New("unsupported Node version")
		}
		check(runtime.name+": Node 24.21.0", err)
		_, err = os.Stat(runtime.script)
		check(runtime.name+": installed source", err)
		_, err = os.Stat(filepath.Join(filepath.Dir(runtime.script), runtime.dependency))
		check(runtime.name+": pinned dependencies", err)
	}
	if failed {
		return errors.New("installation incomplete; install Node 24.21.0 and rerun ./install.sh (see docs/features/installation.md)")
	}
	fmt.Fprintln(out, "Ready to start. Run erii; connect a local model or sign in with ChatGPT in setup.")
	return nil
}
