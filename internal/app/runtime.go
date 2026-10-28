package app

import (
	"context"
	"path/filepath"
	"time"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/agent/sqlite"
	artifacttools "github.com/z-chenhao/erii/internal/artifact/tools"
	"github.com/z-chenhao/erii/internal/automation"
	"github.com/z-chenhao/erii/internal/computer"
	"github.com/z-chenhao/erii/internal/computer/environment"
	computerfake "github.com/z-chenhao/erii/internal/computer/fake"
	"github.com/z-chenhao/erii/internal/computer/macosvm"
	"github.com/z-chenhao/erii/internal/config"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/media/ocr"
	"github.com/z-chenhao/erii/internal/memory"
	"github.com/z-chenhao/erii/internal/recall"
	"github.com/z-chenhao/erii/internal/recall/embedding"
	"github.com/z-chenhao/erii/internal/skill"
	"github.com/z-chenhao/erii/internal/task"
	"github.com/z-chenhao/erii/internal/tool"
	"github.com/z-chenhao/erii/internal/tool/codemode"
	codefixture "github.com/z-chenhao/erii/internal/tool/codemode/fixture"
	toolfake "github.com/z-chenhao/erii/internal/tool/fake"
	"github.com/z-chenhao/erii/internal/tool/mcp"
)

func (a *App) prepareRuntime(ctx context.Context, cfg config.Config, fixture string, secrets llm.SecretStore) error {
	imageOCR := ocr.Discover(ctx)
	registry, err := a.runtimeRegistry(ctx, cfg, fixture, secrets, imageOCR)
	if err != nil {
		return err
	}
	runs := a.store.Runs(a.history, registry)
	local := environment.Local{Config: cfg.ExecutionConfig()}
	runs.ConfigureEnvironment(local.Execute)
	runs.ConfigureImageOCR(imageOCR)
	if cfg.Computer != nil {
		provider, e := macosvm.New(*cfg.Computer, macosvm.SSHGuest{Config: cfg.Computer.Guest, Secrets: secrets})
		if e != nil {
			return e
		}
		runs.ConfigureComputer(provider, cfg.Computer.Compatibility, cfg.Computer.Manifest)
		a.computer = provider
		provider.Boundary = func(point string) {
			if a.testBoundary != nil {
				a.testBoundary("computer_" + point)
			}
		}
	}
	runs.ConfigureContext(cfg.Context)
	if cfg.Memory != nil {
		v := memory.Vault{Secrets: secrets, Ref: cfg.Memory.EncryptionKeyRef}
		runs.ConfigureMemory(a.memory, v)
	}
	if cfg.Embedding != nil {
		runs.ConfigureRecall(recall.Service{Memory: a.memory, Conversation: a.history})
	}
	if cfg.JavaScript != nil {
		var runtime codemode.JavaScriptRuntime = &codemode.Sidecar{Command: cfg.JavaScript.Command, Script: cfg.JavaScript.Script}
		if fixture == "core" {
			runtime = &codefixture.Drift{JavaScriptRuntime: runtime, AuditPath: filepath.Join(cfg.App.DataRoot, "f055-drift.json")}
		}
		runs.ConfigureJavaScript(runtime)
	}
	runs.ConfigureChildLimits(cfg.Children)
	runs.ConfigureScheduler(cfg.Scheduler)
	runs.ConfigureSpend(cfg.Spend)
	runs.ConfigureApprovalTTL(time.Duration(cfg.Tools.ApprovalTTLSeconds) * time.Second)
	runs.ConfigureToolLimits(sqlite.ToolLimits{Global: cfg.Tools.Global, Agent: cfg.Tools.Agent, Run: cfg.Tools.Run, Provider: cfg.Tools.Provider, Tool: cfg.Tools.Tool})
	runs.ConfigureRecentTasks(time.Duration(cfg.Tasks.RecentDays)*24*time.Hour, cfg.Tasks.RecentLimit)
	if a.testAutomationClock != nil {
		runs.SetTestAutomationClock(*a.testAutomationClock)
	}
	a.core.Store(&runtimeCore{runs: runs, registry: registry})
	return nil
}

func (a *App) runtimeRegistry(ctx context.Context, cfg config.Config, fixture string, secrets llm.SecretStore, imageOCR *ocr.Engine) (*tool.Registry, error) {
	var err error
	base := task.Registry()
	if fixture != "" {
		fixtureDefs := task.Definitions()
		echo, _ := tool.FixtureRegistry().Resolve("mcp__fixture__echo")
		base, err = tool.NewRegistry(append(fixtureDefs, echo))
		if err != nil {
			return nil, err
		}
	}
	defs, err := builtinDefinitions(cfg, secrets)
	if err != nil {
		return nil, err
	}
	for _, server := range cfg.MCP {
		more, e := mcp.Definitions(ctx, server)
		if e != nil {
			return nil, e
		}
		defs = append(defs, more...)
	}
	if cfg.Skills != nil {
		more, e := skill.Definitions(*cfg.Skills)
		if e != nil {
			return nil, e
		}
		defs = append(defs, more...)
	}
	if cfg.Skills == nil {
		defs = append(defs, skill.EmptyDefinitions()...)
	}
	identity := any(cfg.ExecutionConfig())
	if cfg.Computer != nil {
		identity = cfg.Computer
	}
	defs = append(defs, environment.Definitions(environment.ResourceHash(identity))...)
	defs = append(defs, ocr.Definition(imageOCR.ResourceHash()))

	if (fixture == "core" || fixture == "codemode") && cfg.JavaScript != nil {
		if cfg.Computer != nil {
			return nil, computer.ErrInvalid
		}
		defs = append(defs, computerfake.Definitions()...)
	}
	if cfg.Computer != nil {
		hash, e := computer.Fingerprint(map[string]any{"configuration": cfg.Computer, "use_runtime": macosvm.UseResourceHash()})
		if e != nil {
			return nil, e
		}
		defs = append(defs, computer.Definitions(hash)...)
	}
	if fixture == "artifacts" {
		defs = append(defs, toolfake.Artifacts()...)
	}
	for _, spec := range base.Describe() {
		d, _ := base.Resolve(spec.Name)
		defs = append(defs, d)
	}
	registry, err := tool.NewRegistry(defs)
	if err != nil {
		return nil, err
	}
	if fixture == "core" || fixture == "codemode" || fixture == "children" || fixture == "approval" || fixture == "approval-never" {
		defs := []tool.Definition{}
		for _, spec := range registry.Describe() {
			d, _ := registry.Resolve(spec.Name)
			defs = append(defs, d)
		}
		policy := "query_before_retry"
		if fixture == "approval-never" {
			policy = "never"
		}
		defs = append(defs, toolfake.Definition(cfg.App.DataRoot, policy, func(name string) {
			if a.testBoundary != nil {
				a.testBoundary(name)
			}
		}))
		registry, err = tool.NewRegistry(defs)
		if err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func builtinDefinitions(cfg config.Config, secrets llm.SecretStore) ([]tool.Definition, error) {
	defs := append(artifacttools.Definitions(), agent.AgentToolDefinition())
	defs = append(defs, automation.Definitions()...)
	manageMemory := memory.Definition()
	if cfg.Memory == nil {
		manageMemory.Spec.Description += " Not configured: requires memory.encryption_key_ref; calls fail with PROVIDER_UNAVAILABLE until configured."
	}
	defs = append(defs, manageMemory)
	recallIdentity := recall.UnconfiguredIdentity
	if cfg.Embedding != nil {
		recallIdentity = (embedding.Client{Config: *cfg.Embedding, Secrets: secrets}).Identity()
	}
	searchMemory := recall.Definition(recallIdentity)
	if cfg.Embedding == nil {
		searchMemory.Spec.Description += " Not configured: requires an explicit embedding provider; calls fail with PROVIDER_UNAVAILABLE until configured."
	}
	defs = append(defs, searchMemory)

	if cfg.JavaScript != nil {
		runtime := &codemode.Sidecar{Command: cfg.JavaScript.Command, Script: cfg.JavaScript.Script}
		runtimeHash, preludeHash, e := runtime.Fingerprint()
		if e != nil {
			return nil, e
		}
		definition := codemode.Definition()
		if cfg.Computer != nil {
			definition.Spec.TimeoutMS = 300000
			definition.Spec.Description += " This installation supports cua.getState/listApps/listWindows/listBrowsers/listTabs, getApp/getBrowser/getTab and explicit createBrowserTab. Read bound documentation(); cua.computer.launchApp({app}) launches, cua.files.importArtifact/exportArtifact transfers scoped references. Every action follows Security and Approval."
		}
		definition.Spec.ResourceHash += ":" + runtimeHash + ":" + preludeHash
		defs = append(defs, definition)
	}
	return defs, nil
}
