package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func noEnv(string) (string, bool) { return "", false }

func TestDefaultsAndDeploymentOverrides(t *testing.T) {
	cfg, err := Load("", noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 1 || !filepath.IsAbs(cfg.App.DataRoot) || cfg.App.LogLevel != "info" {
		t.Fatalf("defaults: %+v", cfg)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("version: 1\napp:\n  data_root: /from-file\n  log_level: warn\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path, func(key string) (string, bool) {
		switch key {
		case "ERII_DATA_ROOT":
			return dir, true
		case "ERII_LOG_LEVEL":
			return "debug", true
		default:
			return "", false
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.App.DataRoot != dir || cfg.App.LogLevel != "debug" {
		t.Fatalf("overrides: %+v", cfg)
	}
}

func TestMCPRejectsDataRootThroughAliasBeforeItExists(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "provider")
	alias := filepath.Join(base, "alias")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "config.yaml")
	body := "version: 1\napp:\n  data_root: " + filepath.Join(alias, "new-data") + "\nmcp:\n  - name: fixture\n    command: /usr/bin/python3\n    directory: " + real + "\n    tools: {read: read_only}\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, noEnv); err == nil {
		t.Fatal("provider can access a future domain data directory through an alias")
	}
}

func TestRejectInvalidConfigWithoutEchoingValues(t *testing.T) {
	for name, body := range map[string]string{
		"mcp-overlapping-providers": "version: 1\nmcp:\n  - name: first\n    command: /usr/bin/python3\n    directory: /provider\n    tools: {read: read_only}\n  - name: second\n    command: /usr/bin/python3\n    directory: /provider/child\n    tools: {read: read_only}",
		"mcp-root-scope":            "version: 1\nmcp:\n  - name: fixture\n    command: /usr/bin/python3\n    directory: /\n    tools: {read: read_only}",
		"workspace-overlaps-data":   "version: 1\napp:\n  data_root: /scope/data\nworkspace:\n  root: /scope\n  mode: edit",
		"workspace-overlaps-skills": "version: 1\nskills:\n  root: /scope/skills\n  scope: agent\nworkspace:\n  root: /scope\n  mode: edit",
		"workspace-invalid-mode":    "version: 1\nworkspace:\n  root: /coding\n  mode: unrestricted",
		"skill-invalid-scope":       "version: 1\nskills:\n  root: /skills\n  scope: all",
		"mcp-overlaps-data":         "version: 1\napp:\n  data_root: /scope/data\nmcp:\n  - name: fixture\n    command: /usr/bin/python3\n    directory: /scope\n    tools: {read: read_only}",
		"mcp-invalid-permission":    "version: 1\nmcp:\n  - name: fixture\n    command: /usr/bin/python3\n    directory: /scope\n    tools: {send: read_only_unrestricted}",
		"delivery-lease":            "version: 1\ndelivery:\n  lease_seconds: 0",
		"delivery-retry":            "version: 1\ndelivery:\n  retry_milliseconds: 0",
		"delivery-attempts":         "version: 1\ndelivery:\n  max_attempts: 21",
		"llm-missing-script":        "version: 1\nllm:\n  provider_bridge_command: SECRET_SENTINEL",
		"llm-relative-script":       "version: 1\nllm:\n  provider_bridge_command: node\n  provider_bridge_script: SECRET_SENTINEL\n  provider_bridge_version: pi-ai-1.3.0-v1",
		"llm-unknown-version":       "version: 1\nllm:\n  provider_bridge_command: node\n  provider_bridge_script: /script.mjs\n  provider_bridge_version: SECRET_SENTINEL",
		"artifact-zero":             "version: 1\nartifact:\n  max_bytes: 0",
		"artifact-quota":            "version: 1\nartifact:\n  quota_bytes: 1",
		"artifact-retention":        "version: 1\nartifact:\n  retention_days: -1",
		"artifact-batch":            "version: 1\nartifact:\n  cleanup_batch: 0",
		"empty":                     "",
		"missing-version":           "app: {}",
		"future-version":            "version: 2",
		"unknown-field":             "version: 1\napi_key: SECRET_SENTINEL",
		"unknown-nested":            "version: 1\napp:\n  password: SECRET_SENTINEL",
		"syntax":                    "version: 1\napp: [SECRET_SENTINEL",
		"bad-type":                  "version: SECRET_SENTINEL",
		"duplicate-field":           "version: 1\nversion: 1",
		"multi-document":            "version: 1\n---\nversion: 1",
		"relative-root":             "version: 1\napp:\n  data_root: relative",
		"bad-log-level":             "version: 1\napp:\n  log_level: SECRET_SENTINEL",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path, noEnv)
			if err == nil {
				t.Fatal("accepted invalid config")
			}
			if strings.Contains(err.Error(), "SECRET_SENTINEL") {
				t.Fatalf("leaked value: %v", err)
			}
		})
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing"), noEnv); err == nil {
		t.Fatal("accepted missing file")
	}
	for _, key := range []string{"ERII_DATA_ROOT", "ERII_LOG_LEVEL"} {
		if _, err := Load("", func(k string) (string, bool) { return "", k == key }); err == nil {
			t.Fatalf("accepted empty %s override", key)
		}
	}
}

func TestNATSConfigAndOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nnats:\n  url: nats://localhost:4222\n  stream_prefix: local\n  timeout: 1s\n  ack_wait: 5s\n  retry_interval: 100ms\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path, func(k string) (string, bool) {
		switch k {
		case "ERII_NATS_URL":
			return "nats://127.0.0.1:4444", true
		case "ERII_NATS_STREAM_PREFIX":
			return "isolated", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.NATS.URL != "nats://127.0.0.1:4444" || cfg.NATS.StreamPrefix != "isolated" {
		t.Fatalf("overrides: %+v", cfg.NATS)
	}
	for _, body := range []string{
		"url: nats://user:SECRET_SENTINEL@localhost:4222",
		"url: nats://localhost:4222?token=SECRET_SENTINEL",
		"url: nats://localhost:4222/SECRET_SENTINEL",
		"url: https://localhost:4222",
		"url: nats://localhost",
		"stream_prefix: x.*",
		"timeout: 0s",
		"timeout: 2m",
		"ack_wait: -1s",
		"retry_interval: 0s",
	} {
		if err := os.WriteFile(path, []byte("version: 1\nnats:\n  "+body+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path, noEnv); err == nil || strings.Contains(err.Error(), "SECRET_SENTINEL") {
			t.Fatalf("NATS config rejected unsafely: %v", err)
		}
	}
	for _, key := range []string{"ERII_NATS_URL", "ERII_NATS_STREAM_PREFIX"} {
		if _, err := Load("", func(k string) (string, bool) { return "", k == key }); err == nil {
			t.Fatalf("empty %s accepted", key)
		}
	}
}
