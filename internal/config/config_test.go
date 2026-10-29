package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func noEnv(string) (string, bool) { return "", false }

func TestSpendRequiresExplicitIntegerPrices(t *testing.T) {
	body := `version: 1
spend:
  accounts:
    - account: openai_compatible:abc
      currency: USD
      boundary_micros: 1000000
      prices:
        - model: fixture
          version: v1
          input_micros_per_million: 0
          output_micros_per_million: 2000000
          cache_read_micros_per_million: 0
          cache_write_micros_per_million: 0
`
	for name, value := range map[string]string{"valid": body, "omitted": strings.Replace(body, "          input_micros_per_million: 0\n", "", 1), "fraction": strings.Replace(body, "input_micros_per_million: 0", "input_micros_per_million: 0.5", 1), "boundary-fraction": strings.Replace(body, "boundary_micros: 1000000", "boundary_micros: 1.5", 1), "unknown": strings.Replace(body, "version: v1", "version: v1\n          secret: PRIVATE", 1), "negative": strings.Replace(body, "input_micros_per_million: 0", "input_micros_per_million: -1", 1)} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(value), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path, noEnv)
			if (err == nil) != (name == "valid") {
				t.Fatal(err)
			}
			if err != nil && strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("configuration leaked")
			}
		})
	}
}

func TestDefaultsAndDeploymentOverrides(t *testing.T) {
	cfg, err := Decode(nil, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(cfg.App.DataRoot) != "collie" || cfg.NATS.StreamPrefix != "collie" || cfg.Computer != nil || cfg.Execution == nil || cfg.Execution.Root != cfg.App.DataRoot+".computer" || cfg.Version != 1 || !filepath.IsAbs(cfg.App.DataRoot) || cfg.App.LogLevel != "info" {
		t.Fatalf("defaults: %+v", cfg)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("version: 1\napp:\n  data_root: /from-file\n  log_level: warn\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path, func(key string) (string, bool) {
		switch key {
		case "COLLIE_DATA_ROOT":
			return dir, true
		case "COLLIE_LOG_LEVEL":
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
		"scheduler-zero":               "version: 1\nscheduler: {child_concurrency: 0}",
		"scheduler-hard-limit":         "version: 1\nscheduler: {child_concurrency: 33}",
		"scheduler-global-hard-limit":  "version: 1\nscheduler: {global_concurrency: 33}",
		"scheduler-user-zero":          "version: 1\nscheduler: {user_concurrency: 0}",
		"scheduler-provider-zero":      "version: 1\nscheduler: {provider_concurrency: 0}",
		"scheduler-agent-zero":         "version: 1\nscheduler: {agent_concurrency: 0}",
		"scheduler-turns-hard-limit":   "version: 1\nscheduler: {max_worker_turns: 513}",
		"scheduler-circuit-zero":       "version: 1\nscheduler: {infrastructure_failures: 0}",
		"scheduler-cooldown-zero":      "version: 1\nscheduler: {cooldown_seconds: 0}",
		"child-depth-zero":             "version: 1\nchildren: {max_depth: 0}",
		"child-depth-hard-limit":       "version: 1\nchildren: {max_depth: 17}",
		"child-descendants-zero":       "version: 1\nchildren: {max_descendants: 0}",
		"child-descendants-hard-limit": "version: 1\nchildren: {max_descendants: 10001}",
		"child-calls-hard-limit":       "version: 1\nchildren: {max_tool_calls: 50001}",
		"child-calls-zero":             "version: 1\nchildren: {max_tool_calls: 0}",
		"mcp-overlapping-providers":    "version: 1\nmcp:\n  - name: first\n    command: /usr/bin/python3\n    directory: /provider\n    tools: {read: read_only}\n  - name: second\n    command: /usr/bin/python3\n    directory: /provider/child\n    tools: {read: read_only}",
		"mcp-root-scope":               "version: 1\nmcp:\n  - name: fixture\n    command: /usr/bin/python3\n    directory: /\n    tools: {read: read_only}",
		"execution-overlaps-data":      "version: 1\napp:\n  data_root: /scope/data\nexecution:\n  root: /scope",
		"execution-overlaps-skills":    "version: 1\nskills:\n  root: /scope/skills\n  scope: agent\nexecution:\n  root: /scope",
		"legacy-workspace-rejected":    "version: 1\nworkspace:\n  root: /coding\n  mode: edit",
		"execution-relative":           "version: 1\nexecution: {root: relative}",
		"execution-toolchain-overlap":  "version: 1\napp: {data_root: /scope/data}\nexecution: {root: /work, toolchain_roots: [/scope]}",
		"skill-invalid-scope":          "version: 1\nskills:\n  root: /skills\n  scope: all",
		"mcp-overlaps-data":            "version: 1\napp:\n  data_root: /scope/data\nmcp:\n  - name: fixture\n    command: /usr/bin/python3\n    directory: /scope\n    tools: {read: read_only}",
		"mcp-invalid-permission":       "version: 1\nmcp:\n  - name: fixture\n    command: /usr/bin/python3\n    directory: /scope\n    tools: {send: read_only_unrestricted}",
		"delivery-lease":               "version: 1\ndelivery:\n  lease_seconds: 0",
		"delivery-retry":               "version: 1\ndelivery:\n  retry_milliseconds: 0",
		"delivery-attempts":            "version: 1\ndelivery:\n  max_attempts: 21",
		"llm-missing-script":           "version: 1\nllm:\n  provider_bridge_command: SECRET_SENTINEL",
		"llm-relative-script":          "version: 1\nllm:\n  provider_bridge_command: node\n  provider_bridge_script: SECRET_SENTINEL\n  provider_bridge_version: pi-ai-1.3.0-v1",
		"llm-unknown-version":          "version: 1\nllm:\n  provider_bridge_command: node\n  provider_bridge_script: /script.mjs\n  provider_bridge_version: SECRET_SENTINEL",
		"artifact-zero":                "version: 1\nartifact:\n  max_bytes: 0",
		"artifact-quota":               "version: 1\nartifact:\n  quota_bytes: 1",
		"artifact-retention":           "version: 1\nartifact:\n  retention_days: -1",
		"artifact-batch":               "version: 1\nartifact:\n  cleanup_batch: 0",
		"empty":                        "",
		"missing-version":              "app: {}",
		"future-version":               "version: 2",
		"unknown-field":                "version: 1\napi_key: SECRET_SENTINEL",
		"unknown-nested":               "version: 1\napp:\n  password: SECRET_SENTINEL",
		"syntax":                       "version: 1\napp: [SECRET_SENTINEL",
		"bad-type":                     "version: SECRET_SENTINEL",
		"duplicate-field":              "version: 1\nversion: 1",
		"multi-document":               "version: 1\n---\nversion: 1",
		"relative-root":                "version: 1\napp:\n  data_root: relative",
		"bad-log-level":                "version: 1\napp:\n  log_level: SECRET_SENTINEL",
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
	for _, key := range []string{"COLLIE_DATA_ROOT", "COLLIE_LOG_LEVEL"} {
		if _, err := Decode(nil, func(k string) (string, bool) { return "", k == key }); err == nil {
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
		case "COLLIE_NATS_URL":
			return "nats://127.0.0.1:4444", true
		case "COLLIE_NATS_STREAM_PREFIX":
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
	for _, key := range []string{"COLLIE_NATS_URL", "COLLIE_NATS_STREAM_PREFIX"} {
		if _, err := Decode(nil, func(k string) (string, bool) { return "", k == key }); err == nil {
			t.Fatalf("empty %s accepted", key)
		}
	}
}

func TestJavaScriptRuntimeRequiresTrustedAbsolutePaths(t *testing.T) {
	for _, yaml := range []string{
		"version: 1\njavascript: {}\n",
		"version: 1\njavascript: {command: node, script: /runtime/sidecar.mjs}\n",
		"version: 1\njavascript: {command: /usr/bin/node, script: sidecar.mjs}\n",
	} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path, func(string) (string, bool) { return "", false }); err == nil {
			t.Fatal("accepted untrusted runtime config", yaml)
		}
	}
}
