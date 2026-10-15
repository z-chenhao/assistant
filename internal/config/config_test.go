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

func TestRejectInvalidConfigWithoutEchoingValues(t *testing.T) {
	for name, body := range map[string]string{
		"empty":           "",
		"missing-version": "app: {}",
		"future-version":  "version: 2",
		"unknown-field":   "version: 1\napi_key: SECRET_SENTINEL",
		"unknown-nested":  "version: 1\napp:\n  password: SECRET_SENTINEL",
		"syntax":          "version: 1\napp: [SECRET_SENTINEL",
		"bad-type":        "version: SECRET_SENTINEL",
		"duplicate-field": "version: 1\nversion: 1",
		"multi-document":  "version: 1\n---\nversion: 1",
		"relative-root":   "version: 1\napp:\n  data_root: relative",
		"bad-log-level":   "version: 1\napp:\n  log_level: SECRET_SENTINEL",
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
