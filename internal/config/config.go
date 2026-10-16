// Package config loads the process configuration. Model credentials and profiles
// are deliberately not part of this foundation's configuration surface.
package config

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	eventnats "github.com/z-chenhao/erii/internal/event/nats"
	"go.yaml.in/yaml/v3"
)

type Config struct {
	Version int               `yaml:"version"`
	App     App               `yaml:"app"`
	NATS    eventnats.Options `yaml:"nats"`
	LLM     LLM               `yaml:"llm"`
}

type LLM struct {
	ProviderBridgeCommand string `yaml:"provider_bridge_command"`
	ProviderBridgeScript  string `yaml:"provider_bridge_script"`
	ProviderBridgeVersion string `yaml:"provider_bridge_version"`
}

type App struct {
	DataRoot string `yaml:"data_root"`
	LogLevel string `yaml:"log_level"`
}

// Load uses built-in defaults when path is empty. A supplied file must exist,
// contain exactly one versioned YAML document, and contain only known fields.
func Load(path string, lookupEnv func(string) (string, bool)) (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, errors.New("resolve home directory")
	}
	cfg := Config{
		Version: 1,
		App:     App{DataRoot: filepath.Join(home, ".local", "share", "erii"), LogLevel: "info"},
		NATS:    eventnats.Options{URL: "nats://127.0.0.1:4222", StreamPrefix: "erii", Timeout: 2 * time.Second, AckWait: 30 * time.Second, RetryInterval: time.Second},
	}
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return Config{}, errors.New("open configuration file")
		}
		defer f.Close()
		decoder := yaml.NewDecoder(f)
		decoder.KnownFields(true)
		// File versions must be explicit, rather than inherited from defaults.
		cfg.Version = 0
		if err := decoder.Decode(&cfg); err != nil {
			return Config{}, errors.New("invalid configuration YAML (fields, types, or syntax)")
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return Config{}, errors.New("configuration must contain exactly one YAML document")
		}
	}
	if value, ok := lookupEnv("ERII_DATA_ROOT"); ok {
		cfg.App.DataRoot = value
	}
	if value, ok := lookupEnv("ERII_LOG_LEVEL"); ok {
		cfg.App.LogLevel = value
	}
	if value, ok := lookupEnv("ERII_NATS_URL"); ok {
		cfg.NATS.URL = value
	}
	if value, ok := lookupEnv("ERII_NATS_STREAM_PREFIX"); ok {
		cfg.NATS.StreamPrefix = value
	}
	if (cfg.LLM.ProviderBridgeCommand == "") != (cfg.LLM.ProviderBridgeScript == "") || cfg.LLM.ProviderBridgeCommand != "" && (!filepath.IsAbs(cfg.LLM.ProviderBridgeScript) || cfg.LLM.ProviderBridgeVersion != "pi-ai-1.3.0-v1") {
		return Config{}, errors.New("llm requires command, absolute script, and supported provider_bridge_version pi-ai-1.3.0-v1")
	}
	if err := cfg.NATS.Validate(); err != nil {
		return Config{}, err
	}
	if cfg.Version != 1 {
		return Config{}, errors.New("unsupported configuration version (expected 1)")
	}
	if !filepath.IsAbs(cfg.App.DataRoot) {
		return Config{}, errors.New("app.data_root must be an absolute path")
	}
	switch cfg.App.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, errors.New("app.log_level must be debug, info, warn, or error")
	}
	cfg.App.DataRoot = filepath.Clean(cfg.App.DataRoot)
	return cfg, nil
}

func (c Config) LogLevel() slog.Level {
	var level slog.Level
	// Load validates the value before this conversion.
	_ = level.UnmarshalText([]byte(c.App.LogLevel))
	return level
}
