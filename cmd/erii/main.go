package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/z-chenhao/erii/internal/app"
	"github.com/z-chenhao/erii/internal/automation"
	"github.com/z-chenhao/erii/internal/computer/macosvm"
	computerfixture "github.com/z-chenhao/erii/internal/computer/macosvm/fixture"
	"github.com/z-chenhao/erii/internal/config"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/llm/fake"
	"github.com/z-chenhao/erii/internal/llm/secret"
	"github.com/z-chenhao/erii/internal/maintenance"
	"github.com/z-chenhao/erii/internal/startup"
	mcpfake "github.com/z-chenhao/erii/internal/tool/mcp/fake"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	args := os.Args[1:]
	if len(args) == 0 && isatty.IsTerminal(os.Stdin.Fd()) {
		args = []string{"start"}
	}
	if err := run(ctx, args, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "erii:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) (err error) {
	if len(args) == 2 && args[0] == "-test-computer-server" {
		return computerfixture.Serve(ctx, args[1], out)
	}
	if len(args) == 2 && args[0] == "-test-mcp-server" {
		return mcpfake.Serve(in, out, args[1])
	}
	if len(args) == 3 && args[0] == "-test-provider-bridge" {
		return fake.ServeRecorded(in, out, args[1], args[2])
	}
	if len(args) == 2 && args[0] == "-test-provider-bridge" {
		return fake.Serve(in, out, args[1])
	}
	flags := flag.NewFlagSet("erii", flag.ContinueOnError)
	flags.SetOutput(errOut)
	configPath := flags.String("config", "", "versioned YAML configuration file (optional)")
	testSecrets := flags.String("test-secret-store", "", "explicit in-memory CI Unix socket; never a production credential backend")
	testBridge := flags.String("test-bridge", "", "explicit deterministic test Bridge fixture; never a real provider")
	testEvents := flags.Bool("test-events", false, "explicit test Bridge only: exercise production NATS lifecycle")
	testDelivery := flags.String("test-delivery", "", "explicit test Bridge only: retry-once, unavailable, ambiguous, unknown")
	testAutomationNow := flags.String("test-automation-now", "", "explicit test Bridge only: fixed Automation clock (RFC3339)")
	testMainHold := flags.String("test-main-hold", "", "explicit test Bridge only: hold at a Main durability boundary until shutdown")
	testMainRelease := flags.String("test-main-release", "", "explicit test Bridge only: release a held boundary when this file exists")
	testBarriers := map[string]string{}
	flags.Func("test-main-barrier", "explicit test Bridge only: repeatable boundary=release-file barrier", func(value string) error {
		name, path, ok := strings.Cut(value, "=")
		if !ok || name == "" || path == "" || testBarriers[name] != "" {
			return errors.New("expected a unique boundary=release-file")
		}
		testBarriers[name] = path
		return nil
	})
	flags.Usage = func() {
		fmt.Fprintln(errOut, "Erii: run erii (or erii start) for guided setup and chat. Run doctor to check the installation. Ctrl-C or /quit stops Erii.\nstart manages a private local broker; -config or ERII_NATS_URL uses your configured broker instead.")
		fmt.Fprintln(errOut, "Operations: ops health|report|broker|upgrade-broker; ops trace RUN_ID; ops retry-event EVENT_ID ATTEMPTS; ops backup ARCHIVE KEY_REF; ops restore ARCHIVE KEY_REF; ops activate source-retired-and-outcomes-reconciled. See docs/features/operations.md.")
		fmt.Fprintln(errOut, "Usage: erii [-config path] [-test-bridge fixture] [web serve|channel manage|tui|status|relay|automation manage REQUEST_KEY|automation list|automation tick|automation work|computer status|computer prepare-base CLEAN_STOPPED_VM|delivery list|delivery work|memory manage REQUEST_KEY|memory work|memory status|skill manage|chat send [REQUEST_KEY]|chat send-parts [REQUEST_KEY]|artifact save NAME MEDIA_TYPE|artifact open REF|artifact describe REF|artifact delete REF VERSION|artifact cleanup [apply]|chat work|chat history|spend status|approval list|approval approve ID HASH|approval deny ID HASH|approval reconcile|model add [URL MODEL_ID [AUTH_ID]]|model discover URL [AUTH_ID]|model list|model catalog [PROFILE_ID]|model login [ENV_NAME]|model test ID|model select ID PROFILE_VERSION CONFIG_VERSION|model cancel]\nWithout a command, open the TUI on a terminal; with redirected stdin, print diagnostics and wait for SIGINT/SIGTERM.\nrelay runs the internal event publisher until stopped.\nmemory manage and skill manage read one control JSON object on stdin; memory work drains a bounded maintenance batch.\nchat send reads text on stdin and durably accepts it; chat work drains/reclaims Main work in the foreground; chat history reads semantic messages.\nmodel add reads metadata JSON; test proves capabilities; select atomically activates a tested version. model cancel leaves setup unchanged. ChatGPT: model login-chatgpt [ACCOUNT_ID], accounts, account-catalog ACCOUNT_ID, account-add ACCOUNT_ID MODEL_ID, refresh ACCOUNT_ID, logout ACCOUNT_ID. Login emits a private browser URL; open it to continue with ChatGPT.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if len(testBarriers) > 0 && *testBridge == "" {
		return errors.New("test-main-barrier requires explicit test-bridge")
	}
	if *testEvents && *testBridge == "" {
		return errors.New("test-events requires explicit test-bridge")
	}
	if testBarriers[*testMainHold] != "" {
		return errors.New("conflicting test boundary barriers")
	}
	modelCommand := false
	if flags.Arg(0) == "model" {
		switch flags.Arg(1) {
		case "add":
			modelCommand = flags.NArg() == 2 || flags.NArg() == 4 || flags.NArg() == 5
		case "discover":
			modelCommand = flags.NArg() == 3 || flags.NArg() == 4
		case "list", "cancel", "accounts":
			modelCommand = flags.NArg() == 2
		case "catalog", "login", "login-chatgpt":
			modelCommand = flags.NArg() == 2 || flags.NArg() == 3
		case "test", "refresh", "logout", "account-catalog":
			modelCommand = flags.NArg() == 3
		case "account-add":
			modelCommand = flags.NArg() == 4
		case "select":
			modelCommand = flags.NArg() == 5
		}
	}
	chatCommand := flags.Arg(0) == "chat" && (((flags.Arg(1) == "send" || flags.Arg(1) == "send-parts") && (flags.NArg() == 2 || flags.NArg() == 3)) || ((flags.Arg(1) == "work" || flags.Arg(1) == "history") && flags.NArg() == 2))
	artifactCommand := flags.Arg(0) == "artifact" && ((flags.NArg() == 4 && (flags.Arg(1) == "save" || flags.Arg(1) == "delete")) || (flags.NArg() == 3 && (flags.Arg(1) == "open" || flags.Arg(1) == "describe")) || (flags.Arg(1) == "cleanup" && (flags.NArg() == 2 || (flags.NArg() == 3 && flags.Arg(2) == "apply"))))
	deliveryCommand := flags.Arg(0) == "delivery" && flags.NArg() == 2 && (flags.Arg(1) == "work" || flags.Arg(1) == "list")
	approvalCommand := flags.Arg(0) == "approval" && ((flags.NArg() == 2 && (flags.Arg(1) == "list" || flags.Arg(1) == "reconcile")) || (flags.NArg() == 4 && (flags.Arg(1) == "approve" || flags.Arg(1) == "deny")))
	startCommand := flags.NArg() == 1 && flags.Arg(0) == "start"
	doctorCommand := flags.NArg() == 1 && flags.Arg(0) == "doctor"
	terminalCommand := startCommand || flags.NArg() == 1 && flags.Arg(0) == "tui"
	if startCommand {
		if f, ok := in.(*os.File); !ok || !isatty.IsTerminal(f.Fd()) {
			return errors.New("start needs an interactive terminal; use status for diagnostics or web serve for the browser interface")
		}
	}
	if f, ok := in.(*os.File); ok && flags.NArg() == 0 && isatty.IsTerminal(f.Fd()) {
		terminalCommand = true
	}
	skillCommand := flags.Arg(0) == "skill" && flags.Arg(1) == "manage" && flags.NArg() == 2
	memoryCommand := flags.Arg(0) == "memory" && ((flags.NArg() == 2 && (flags.Arg(1) == "work" || flags.Arg(1) == "status")) || (flags.NArg() == 3 && flags.Arg(1) == "manage"))
	spendCommand := flags.NArg() == 2 && flags.Arg(0) == "spend" && flags.Arg(1) == "status"
	computerCommand := flags.Arg(0) == "computer" && ((flags.NArg() == 2 && flags.Arg(1) == "status") || (flags.NArg() == 3 && flags.Arg(1) == "prepare-base"))
	webCommand := flags.NArg() == 2 && flags.Arg(0) == "web" && flags.Arg(1) == "serve"
	channelCommand := flags.NArg() == 2 && flags.Arg(0) == "channel" && flags.Arg(1) == "manage"
	automationCommand := flags.Arg(0) == "automation" && ((flags.NArg() == 3 && flags.Arg(1) == "manage") || (flags.NArg() == 2 && (flags.Arg(1) == "list" || flags.Arg(1) == "tick" || flags.Arg(1) == "work")))
	opsCommand := flags.Arg(0) == "ops" && ((flags.NArg() == 2 && (flags.Arg(1) == "health" || flags.Arg(1) == "report" || flags.Arg(1) == "broker" || flags.Arg(1) == "upgrade-broker")) || (flags.NArg() == 3 && (flags.Arg(1) == "trace" || flags.Arg(1) == "activate")) || (flags.NArg() == 4 && (flags.Arg(1) == "backup" || flags.Arg(1) == "restore" || flags.Arg(1) == "retry-event")))
	if !doctorCommand && !opsCommand && !webCommand && !channelCommand && !automationCommand && !computerCommand && !skillCommand && !memoryCommand && !spendCommand && !deliveryCommand && !artifactCommand && !approvalCommand && !modelCommand && !chatCommand && !terminalCommand && (flags.NArg() > 1 || (flags.NArg() == 1 && flags.Arg(0) != "status" && flags.Arg(0) != "relay")) {
		return errors.New("expected status, relay, model add/list/catalog/test/select/cancel, or no command (see -help)")
	}
	var input llm.ProfileInput
	if modelCommand && flags.Arg(1) == "add" && flags.NArg() >= 4 {
		input = llm.ProfileInput{Name: flags.Arg(3), ProviderKind: "openai_compatible", Endpoint: flags.Arg(2), WireAPI: "openai_completions", ModelID: flags.Arg(3), ContextWindow: 8192, MaxOutputTokens: 1024, Compatibility: llm.Compatibility{ToolResultFormat: "openai"}, Capabilities: llm.Capabilities{TextInput: true}}
		if flags.NArg() == 5 {
			input.AuthConnectionID = flags.Arg(4)
		}
		if err := input.Validate(); err != nil {
			return err
		}
	}
	if modelCommand && flags.Arg(1) == "add" && flags.NArg() == 2 {
		data, err := io.ReadAll(io.LimitReader(in, 65537))
		if err != nil || len(data) > 65536 {
			return errors.New("expected one model metadata object of at most 64 KiB")
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			return errors.New("invalid model metadata JSON (unknown fields, types, or syntax)")
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return errors.New("expected one model metadata object of at most 64 KiB")
		}
		if err := input.Validate(); err != nil {
			return err
		}
	}
	cfg, err := config.Load(*configPath, os.LookupEnv)
	if err != nil {
		return err
	}
	// Bundled defaults belong to the user startup/diagnostic path. Execution
	// commands still require their explicit configuration; never attach a new
	// provider bridge while reclaiming an existing Run.
	if (startCommand || doctorCommand || flags.Arg(0) == "status") && *configPath == "" && *testBridge == "" {
		home, err := runtimeHome()
		if err != nil {
			return err
		}
		configureRuntime(&cfg, home)
	}
	if doctorCommand {
		return doctor(ctx, cfg, out)
	}
	if startCommand {
		if *configPath == "" && *testBridge == "" {
			var diagnostics bytes.Buffer
			if err := doctor(ctx, cfg, &diagnostics); err != nil {
				fmt.Fprint(errOut, diagnostics.String())
				return err
			}
		}
		if _, external := os.LookupEnv("ERII_NATS_URL"); *configPath == "" && !external {
			broker, err := startup.OpenLocalBroker(ctx, cfg.App.DataRoot)
			if err != nil {
				return err
			}
			defer broker.Close()
			cfg.NATS.URL = broker.URL()
		}
	}
	if opsCommand {
		if flags.Arg(1) == "activate" {
			if err = maintenance.Activate(cfg.App.DataRoot, flags.Arg(2)); err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(map[string]bool{"activated": true})
		}
		if flags.Arg(1) == "backup" || flags.Arg(1) == "restore" {
			var secrets llm.SecretStore = secret.Store{}
			if *testSecrets != "" {
				secrets = secret.TestSocket{Path: *testSecrets}
			}
			key, e := secrets.Get(ctx, flags.Arg(3))
			if e != nil || len(key) != 32 {
				return errors.New("backup requires a 32-byte SecretStore key")
			}
			defer func() {
				for i := range key {
					key[i] = 0
				}
			}()
			var manifest maintenance.Manifest
			if flags.Arg(1) == "backup" {
				manifest, err = maintenance.Backup(ctx, cfg.App.DataRoot, flags.Arg(2), key, app.SchemaVersions())
			} else {
				manifest, err = maintenance.Restore(ctx, flags.Arg(2), cfg.App.DataRoot, key, app.SchemaVersions())
			}
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(manifest)
		}
	}
	if computerCommand && flags.Arg(1) == "prepare-base" {
		if cfg.Computer == nil {
			return errors.New("computer configuration required")
		}
		if err := macosvm.ProvisionBase(ctx, cfg.Computer.Root, flags.Arg(2), cfg.Computer.Compatibility); err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(map[string]string{"base": "sealed"})
	}
	var application *app.App
	if (*testBridge == "" || *testEvents) && (webCommand || terminalCommand || automationCommand && flags.Arg(1) == "work") {
		application, err = app.OpenRuntime(ctx, cfg, *testBridge, *testSecrets)
	} else if modelCommand || opsCommand || flags.Arg(0) == "status" {
		application, err = app.OpenControl(ctx, cfg, *testBridge, *testSecrets)
	} else if *testBridge != "" {
		application, err = app.OpenWithTestBridge(ctx, cfg, *testBridge)
	} else if *testSecrets != "" {
		application, err = app.OpenWithTestSecrets(ctx, cfg, *testSecrets)
	} else {
		application, err = app.Open(ctx, cfg)
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, application.Close()) }()
	if opsCommand {
		switch flags.Arg(1) {
		case "health", "report":
			v, e := application.Operations(ctx)
			if e != nil {
				return e
			}
			return json.NewEncoder(out).Encode(v)
		case "trace":
			v, e := application.OperationalTrace(ctx, flags.Arg(2))
			if e != nil {
				return e
			}
			return json.NewEncoder(out).Encode(v)
		case "broker":
			v, e := application.BrokerHealth(ctx, cfg)
			if e != nil {
				return e
			}
			return json.NewEncoder(out).Encode(v)
		case "upgrade-broker":
			v, e := application.UpgradeBroker(ctx, cfg)
			if e != nil {
				return e
			}
			return json.NewEncoder(out).Encode(v)
		case "retry-event":
			n, e := strconv.Atoi(flags.Arg(3))
			if e != nil || n < 0 {
				return errors.New("expected non-negative event attempt count")
			}
			if e = application.RetryEvent(ctx, flags.Arg(2), n); e != nil {
				return e
			}
			return json.NewEncoder(out).Encode(map[string]bool{"requeued": true})
		}
	}
	if *testAutomationNow != "" {
		if *testBridge == "" {
			return errors.New("test-automation-now requires explicit test-bridge")
		}
		now, e := time.Parse(time.RFC3339, *testAutomationNow)
		if e != nil {
			return automation.ErrInvalid
		}
		application.SetTestAutomationClock(now)
	}
	if computerCommand {
		state, err := application.ComputerStatus(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(state)
	}
	if *testDelivery != "" {
		if *testBridge == "" {
			return errors.New("test-delivery requires explicit test-bridge")
		}
		if err := application.SetTestDelivery(*testDelivery); err != nil {
			return err
		}
	}
	if *testMainHold != "" || len(testBarriers) > 0 {
		if *testBridge == "" {
			return errors.New("test-main-hold requires explicit test-bridge")
		}
		application.SetTestMainBoundary(func(name string) {
			release, held := testBarriers[name]
			if name == *testMainHold {
				release, held = *testMainRelease, true
			}
			if held {
				_ = json.NewEncoder(out).Encode(struct {
					Boundary string `json:"boundary"`
				}{name})
				if release == "" {
					<-ctx.Done()
				} else {
					ticker := time.NewTicker(10 * time.Millisecond)
					defer ticker.Stop()
					for {
						if _, err := os.Stat(release); err == nil {
							break
						}
						select {
						case <-ctx.Done():
							return
						case <-ticker.C:
						}
					}
				}
			}
		})
	}
	foreground := func(action func(context.Context) error) error {
		if *testBridge == "" || *testEvents {
			return application.WithEvents(ctx, cfg, action)
		}
		application.StartMemory(ctx)
		return action(ctx)
	}
	if !terminalCommand && flags.NArg() == 0 || chatCommand && flags.Arg(1) == "work" {
		application.StartMemory(ctx)
	}
	if webCommand {
		return runWeb(ctx, application, cfg, out, foreground)
	}
	if channelCommand {
		return runChannel(ctx, application, flags, in, out)
	}
	if deliveryCommand {
		return runDelivery(ctx, application, flags, in, out)
	}
	if automationCommand {
		return runAutomation(ctx, application, flags, in, out, foreground)
	}
	if artifactCommand {
		return runArtifact(ctx, application, flags, in, out)
	}
	if skillCommand {
		return runSkill(ctx, application, flags, in, out)
	}
	if memoryCommand {
		return runMemory(ctx, application, flags, in, out)
	}
	if spendCommand {
		values, err := application.Spend(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(values)
	}
	if approvalCommand {
		return runApproval(ctx, application, flags, in, out)
	}

	if terminalCommand {
		return runTerminal(ctx, application, in, out, foreground)
	}
	if chatCommand {
		return runChat(ctx, application, flags, in, out)
	}
	if modelCommand {
		return runModel(ctx, application, flags, in, out, input, *testBridge != "", errOut)
	}
	diagnostics, err := application.Diagnostics(ctx)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(out).Encode(diagnostics); err != nil {
		return err
	}
	if flags.NArg() == 1 && flags.Arg(0) == "relay" {
		return application.Relay(ctx, cfg)
	}
	if flags.NArg() == 0 {
		<-ctx.Done()
	}
	return nil
}
