// Package config loads the process configuration. Model credentials and profiles
// are deliberately not part of this foundation's configuration surface.
package config

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/z-chenhao/erii/internal/artifact"
	"github.com/z-chenhao/erii/internal/computer/workspace"
	"github.com/z-chenhao/erii/internal/delivery"
	eventnats "github.com/z-chenhao/erii/internal/event/nats"
	"github.com/z-chenhao/erii/internal/skill"
	"github.com/z-chenhao/erii/internal/tool/mcp"
	"go.yaml.in/yaml/v3"
)

type Config struct {
	MCP       []mcp.Config      `yaml:"mcp"`
	Skills    *skill.Config     `yaml:"skills"`
	Workspace *workspace.Config `yaml:"workspace"`
	Delivery  delivery.Policy   `yaml:"delivery"`
	Version   int               `yaml:"version"`
	Artifact  artifact.Policy   `yaml:"artifact"`
	App       App               `yaml:"app"`
	NATS      eventnats.Options `yaml:"nats"`
	Tools     ToolLimits        `yaml:"tools"`
	Tasks     Tasks             `yaml:"tasks"`
	LLM       LLM               `yaml:"llm"`
}

type ToolLimits struct {
	ApprovalTTLSeconds int `yaml:"approval_ttl_seconds"`
	Global             int `yaml:"global_concurrency"`
	Agent              int `yaml:"agent_concurrency"`
	Run                int `yaml:"run_concurrency"`
	Provider           int `yaml:"provider_concurrency"`
	Tool               int `yaml:"tool_concurrency"`
}

type Tasks struct {
	RecentDays  int `yaml:"recent_days"`
	RecentLimit int `yaml:"recent_limit"`
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
		Version:  1,
		Delivery: delivery.DefaultPolicy(),
		Artifact: artifact.DefaultPolicy(),
		Tools:    ToolLimits{ApprovalTTLSeconds: 3600, Global: 32, Agent: 24, Run: 4, Provider: 16, Tool: 8},
		Tasks:    Tasks{RecentDays: 30, RecentLimit: 200},
		App:      App{DataRoot: filepath.Join(home, ".local", "share", "erii"), LogLevel: "info"},
		NATS:     eventnats.Options{URL: "nats://127.0.0.1:4222", StreamPrefix: "erii", Timeout: 2 * time.Second, AckWait: 30 * time.Second, RetryInterval: time.Second},
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
	if cfg.Delivery.Validate() != nil {
		return Config{}, errors.New("invalid delivery lease/retry/attempt policy")
	}
	if cfg.Artifact.Validate() != nil {
		return Config{}, errors.New("invalid artifact retention/quota/size/batch policy")
	}
	if err := cfg.NATS.Validate(); err != nil {
		return Config{}, err
	}
	if cfg.Tools.ApprovalTTLSeconds < 1 || cfg.Tools.ApprovalTTLSeconds > 86400 {
		return Config{}, errors.New("tools approval_ttl_seconds must be 1..86400")
	}
	for _, limit := range []int{cfg.Tools.Global, cfg.Tools.Agent, cfg.Tools.Run, cfg.Tools.Provider, cfg.Tools.Tool} {
		if limit < 1 || limit > 1024 {
			return Config{}, errors.New("tools concurrency limits must be 1..1024")
		}
	}
	if cfg.Tasks.RecentDays < 1 || cfg.Tasks.RecentDays > 3650 || cfg.Tasks.RecentLimit < 1 || cfg.Tasks.RecentLimit > 10000 {
		return Config{}, errors.New("tasks requires recent_days 1..3650 and recent_limit 1..10000")
	}
	if cfg.Version != 1 {
		return Config{}, errors.New("unsupported configuration version (expected 1)")
	}
	if !filepath.IsAbs(cfg.App.DataRoot) {
		return Config{}, errors.New("app.data_root must be an absolute path")
	}
	seenServers := map[string]bool{}
	for _, server := range cfg.MCP {
		if server.Validate() != nil || seenServers[server.Name] || overlaps(server.Directory, cfg.App.DataRoot) || (cfg.Skills != nil && overlaps(server.Directory, cfg.Skills.Root)) || (cfg.Workspace != nil && overlaps(server.Directory, cfg.Workspace.Root)) {
			return Config{}, errors.New("invalid or duplicate MCP configuration")
		}
		seenServers[server.Name] = true
	}
	if cfg.Skills != nil && cfg.Skills.Validate() != nil {
		return Config{}, errors.New("invalid Skills configuration")
	}
	if cfg.Workspace != nil {
		if cfg.Workspace.Validate() != nil || overlaps(cfg.Workspace.Root, cfg.App.DataRoot) {
			return Config{}, errors.New("workspace must be separate from application data")
		}
		if cfg.Skills != nil && overlaps(cfg.Workspace.Root, cfg.Skills.Root) {
			return Config{}, errors.New("workspace cannot contain or overlap Skills")
		}
		for _, server := range cfg.MCP {
			if overlaps(cfg.Workspace.Root, server.Command) {
				return Config{}, errors.New("workspace cannot contain trusted MCP executable")
			}
		}
	}
	switch cfg.App.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, errors.New("app.log_level must be debug, info, warn, or error")
	}
	cfg.App.DataRoot = filepath.Clean(cfg.App.DataRoot)
	return cfg, nil
}

func overlaps(a, b string) bool {
	// Resolve existing aliases before comparing; nonexistent paths are checked
	// again by the adapters when opened.
	if p, e := filepath.EvalSymlinks(a); e == nil {
		a = p
	}
	if p, e := filepath.EvalSymlinks(b); e == nil {
		b = p
	}
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	within := func(parent, child string) bool {
		relative, err := filepath.Rel(parent, child)
		return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
	}
	return within(a, b) || within(b, a)
}

func (c Config) LogLevel() slog.Level {
	var level slog.Level
	// Load validates the value before this conversion.
	_ = level.UnmarshalText([]byte(c.App.LogLevel))
	return level
}
