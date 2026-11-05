package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/config"
)

func TestStartRejectsRedirectedInputBeforeCreatingState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	t.Setenv("STEWARD_DATA_ROOT", root)
	var out bytes.Buffer
	err := run(context.Background(), []string{"start"}, strings.NewReader(""), &out, &out)
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatal("start silently became a diagnostic daemon", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("redirected startup created state", err)
	}
}

func TestDoctorReportsMissingInstallationWithoutCreatingState(t *testing.T) {
	home := t.TempDir()
	cfg, err := config.Decode(nil, func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	configureRuntime(&cfg, home)
	// A deterministic executable version probe; no installed Node needed here.
	command := filepath.Join(home, "node")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nprintf 'v24.21.0\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg.JavaScript.Command = command
	var out bytes.Buffer
	if err := doctor(context.Background(), cfg, &out); err == nil || !strings.Contains(out.String(), "FAIL model bridge: native Go protocol") {
		t.Fatal("doctor accepted missing installation", out.String(), err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.LLM.ProviderBridgeCommand), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.LLM.ProviderBridgeCommand, []byte("#!/bin/sh\nprintf 'steward-go-v1\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{cfg.JavaScript.Script,
		filepath.Join(home, "runtime/javascript/node_modules/quickjs-wasi/package.json")} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	out.Reset()
	if err := doctor(context.Background(), cfg, &out); err != nil || strings.Contains(out.String(), "FAIL") {
		t.Fatal("complete installation rejected", out.String(), err)
	}
	// A configured deployment keeps its own trusted runtime paths.
	configureRuntime(&cfg, "/different-install")
	if cfg.LLM.ProviderBridgeCommand != filepath.Join(home, "bin", "steward") || cfg.JavaScript.Command != command {
		t.Fatal("startup replaced configured runtime")
	}
}

func TestDoctorRespectsDisabledOptionalRuntimes(t *testing.T) {
	var out bytes.Buffer
	if err := doctor(context.Background(), config.Config{}, &out); err != nil {
		t.Fatal("explicit config without optional runtimes panicked or enabled them", err)
	}
	if !strings.Contains(out.String(), "SKIP model bridge") {
		t.Fatal("missing configuration was hidden", out.String())
	}
}

func TestUnconfiguredDrainDoesNotInferAnInstalledProvider(t *testing.T) {
	t.Setenv("STEWARD_DATA_ROOT", t.TempDir())
	var out bytes.Buffer
	err := run(context.Background(), []string{"chat", "work"}, strings.NewReader(""), &out, &out)
	if !errors.Is(err, agent.ErrSetup) || out.Len() != 0 {
		t.Fatal("unconfigured execution silently gained a provider", out.String(), err)
	}
}
