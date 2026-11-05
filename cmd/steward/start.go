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

	"github.com/z-chenhao/assistant/internal/computer/environment"
	"github.com/z-chenhao/assistant/internal/config"
	"github.com/z-chenhao/assistant/internal/llm/bridge"
	"github.com/z-chenhao/assistant/internal/llm/secret"
	"github.com/z-chenhao/assistant/internal/media/ocr"
)

// A source build and an installed bundle share the same relative layout. Use
// the executable, never cwd: users can launch Steward from any working directory.
func runtimeHome() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", errors.New("locate Steward installation")
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", errors.New("resolve Steward installation")
	}
	return filepath.Dir(filepath.Dir(path)), nil
}

func configureRuntime(cfg *config.Config, home string) {
	if cfg.LLM.ProviderBridgeCommand == "" {
		cfg.LLM = config.LLM{ProviderBridgeCommand: filepath.Join(home, "bin", "steward"), ProviderBridgeScript: "-model-bridge", ProviderBridgeVersion: bridge.Version}
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
		if cfg.LLM.ProviderBridgeVersion == bridge.Version {
			value, err := exec.CommandContext(ctx, cfg.LLM.ProviderBridgeCommand, "-model-bridge-version").Output()
			if err == nil && strings.TrimSpace(string(value)) != bridge.Version {
				err = errors.New("unsupported native model bridge")
			}
			check("model bridge: native Go protocol", err)
		} else {
			value, err := exec.LookPath(cfg.LLM.ProviderBridgeCommand)
			check("model bridge: configured command", err)
			_ = value
			_, err = os.Stat(cfg.LLM.ProviderBridgeScript)
			check("model bridge: installed source", err)
		}
	} else {
		fmt.Fprintln(out, "SKIP model bridge: not configured; use default steward startup or add llm settings")
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
	if cfg.Computer == nil {
		check("file/command environment: system Python 3, bash and kernel sandbox", environment.Check())
	} else {
		fmt.Fprintln(out, "SKIP local execution: file/command tools use the configured Computer Guest")
	}
	imageOCR := ocr.Discover(ctx)
	if imageOCR.Available() {
		fmt.Fprintln(out, "OK   image OCR: local Tesseract; languages="+strings.Join(imageOCR.Languages(), ","))
	} else {
		fmt.Fprintln(out, "SKIP image OCR: install Tesseract, eng/chi_sim/chi_tra language data and the kernel sandbox; see docs/features/artifacts-media.md")
	}
	backend, available := secret.NativeBackend()
	if available {
		fmt.Fprintln(out, "OK   credential backend library:", backend, "(login keyring access is checked when used)")
	} else {
		fmt.Fprintln(out, "SKIP native credentials:", backend, "unavailable; on Linux install libsecret-1-0 and use a login session keyring; see docs/features/model-connections.md")
	}
	if failed {
		return errors.New("installation incomplete; check Node 24.21.0 and execution dependencies (Python 3, bash, bubblewrap on Linux), then rerun ./install.sh (see docs/features/installation.md)")
	}
	fmt.Fprintln(out, "Ready to start. Run steward; connect a local model or sign in with ChatGPT in setup.")
	return nil
}
