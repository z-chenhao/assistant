package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/z-chenhao/erii/internal/interaction/web"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/mattn/go-isatty"
	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/app"
	"github.com/z-chenhao/erii/internal/artifact"
	"github.com/z-chenhao/erii/internal/automation"
	"github.com/z-chenhao/erii/internal/computer/macosvm"
	computerfixture "github.com/z-chenhao/erii/internal/computer/macosvm/fixture"
	"github.com/z-chenhao/erii/internal/config"
	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/delivery"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/llm/fake"
	"github.com/z-chenhao/erii/internal/memory"
	"github.com/z-chenhao/erii/internal/skill"
	mcpfake "github.com/z-chenhao/erii/internal/tool/mcp/fake"
	"github.com/z-chenhao/erii/internal/tui"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "personal-agent:", err)
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
	flags := flag.NewFlagSet("personal-agent", flag.ContinueOnError)
	flags.SetOutput(errOut)
	configPath := flags.String("config", "", "versioned YAML configuration file (optional)")
	testSecrets := flags.String("test-secret-store", "", "explicit in-memory CI Unix socket; never a production credential backend")
	testBridge := flags.String("test-bridge", "", "explicit deterministic test Bridge fixture; never a real provider")
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
		fmt.Fprintln(errOut, "Usage: personal-agent [-config path] [-test-bridge fixture] [web serve|channel manage|tui|status|relay|automation manage REQUEST_KEY|automation list|automation tick|automation work|computer status|computer prepare-base CLEAN_STOPPED_VM|delivery list|delivery work|memory manage REQUEST_KEY|memory work|memory status|skill manage|chat send [REQUEST_KEY]|chat send-parts [REQUEST_KEY]|artifact save NAME MEDIA_TYPE|artifact open REF|artifact describe REF|artifact delete REF VERSION|artifact cleanup [apply]|chat work|chat history|spend status|approval list|approval approve ID HASH|approval deny ID HASH|approval reconcile|model add [URL MODEL_ID [AUTH_ID]]|model discover URL [AUTH_ID]|model list|model catalog [PROFILE_ID]|model login [ENV_NAME]|model test ID|model select ID PROFILE_VERSION CONFIG_VERSION|model cancel]\nWithout a command, open the TUI on a terminal; with redirected stdin, print diagnostics and wait for SIGINT/SIGTERM.\nrelay runs the internal event publisher until stopped.\nmemory manage and skill manage read one control JSON object on stdin; memory work drains a bounded maintenance batch.\nchat send reads text on stdin and durably accepts it; chat work drains/reclaims Main work in the foreground; chat history reads semantic messages.\nmodel add reads metadata JSON; test proves capabilities; select atomically activates a tested version. model cancel leaves setup unchanged. ChatGPT: model login-chatgpt [ACCOUNT_ID], accounts, account-catalog ACCOUNT_ID, account-add ACCOUNT_ID MODEL_ID, refresh ACCOUNT_ID, logout ACCOUNT_ID. Login emits a private browser URL; open it to continue with ChatGPT.")
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
	terminalCommand := flags.NArg() == 1 && flags.Arg(0) == "tui"
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
	if !webCommand && !channelCommand && !automationCommand && !computerCommand && !skillCommand && !memoryCommand && !spendCommand && !deliveryCommand && !artifactCommand && !approvalCommand && !modelCommand && !chatCommand && !terminalCommand && (flags.NArg() > 1 || (flags.NArg() == 1 && flags.Arg(0) != "status" && flags.Arg(0) != "relay")) {
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
	if *testBridge != "" {
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
	if webCommand || terminalCommand || flags.NArg() == 0 || chatCommand && flags.Arg(1) == "work" {
		application.StartMemory(ctx)
	}
	if webCommand {
		if cfg.Web == nil {
			return errors.New("web configuration is required")
		}
		bindings, err := application.ChannelBindings(ctx)
		if err != nil {
			return err
		}
		found := false
		for _, b := range bindings {
			if b.Channel == "web" && b.Subject == web.Subject && b.Destination == web.Destination {
				found = true
			}
		}
		if !found {
			if _, err := application.BindChannel(ctx, "web", web.Subject, web.Destination, true, 0); err != nil {
				return err
			}
		}
		listener, err := net.Listen("tcp", cfg.Web.Listen)
		if err != nil {
			return errors.New("web listener unavailable")
		}
		defer listener.Close()
		if err = json.NewEncoder(out).Encode(map[string]string{"url": "http://" + listener.Addr().String()}); err != nil {
			return err
		}
		return application.ServeWeb(ctx, listener, *cfg.Web)
	}
	if channelCommand {
		var q struct {
			Op          string `json:"op"`
			Channel     string `json:"channel"`
			Subject     string `json:"subject"`
			Destination string `json:"destination"`
			Enabled     bool   `json:"enabled"`
			Version     int64  `json:"version"`
			BindingID   string `json:"binding_id"`
		}
		dec := json.NewDecoder(io.LimitReader(in, 64<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&q); err != nil {
			return errors.New("invalid channel control request")
		}
		return application.ManageChannel(ctx, q.Op, q.Channel, q.Subject, q.Destination, q.Enabled, q.Version, q.BindingID, out)
	}
	if deliveryCommand {
		if flags.Arg(1) == "work" {
			if err := application.Deliver(ctx); err != nil {
				return err
			}
		}
		requests, err := application.DeliveryRequests(ctx)
		if err != nil {
			return err
		}
		// The CLI protocol uses RFC 3339 UTC; SQLite keeps UTC microseconds.
		views := make([]struct {
			delivery.Request
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
		}, len(requests))
		for i, r := range requests {
			views[i].Request = r
			views[i].CreatedAt = time.UnixMicro(r.CreatedAt).UTC()
			views[i].UpdatedAt = time.UnixMicro(r.UpdatedAt).UTC()
		}
		return json.NewEncoder(out).Encode(views)
	}
	if automationCommand {
		if flags.Arg(1) == "work" {
			return application.WorkAutomations(ctx)
		}
		if flags.Arg(1) == "tick" {
			if e := application.ScheduleAutomations(ctx); e != nil {
				return e
			}
			return json.NewEncoder(out).Encode(map[string]bool{"accepted": true})
		}
		q := automation.Request{Operation: "list"}
		if flags.Arg(1) == "manage" {
			raw, e := io.ReadAll(io.LimitReader(in, 16385))
			if e != nil || len(raw) > 16384 {
				return automation.ErrInvalid
			}
			q, e = automation.Parse(string(raw))
			if e != nil {
				return e
			}
		}
		key := flags.Arg(2)
		if key == "" {
			key = "list"
		}
		result, e := application.ManageAutomation(ctx, key, q)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(result)
	}
	if artifactCommand {
		switch flags.Arg(1) {
		case "save":
			d, e := application.SaveArtifact(ctx, artifact.Input{Name: flags.Arg(2), MediaType: flags.Arg(3)}, in)
			if e != nil {
				return e
			}
			return json.NewEncoder(out).Encode(d)
		case "open":
			f, e := application.OpenArtifact(ctx, flags.Arg(2))
			if e != nil {
				return e
			}
			defer f.Close()
			_, e = io.Copy(out, f)
			return e
		case "describe":
			d, e := application.DescribeArtifact(ctx, flags.Arg(2))
			if e != nil {
				return e
			}
			return json.NewEncoder(out).Encode(d)
		case "delete":
			version, e := strconv.ParseInt(flags.Arg(3), 10, 64)
			if e != nil {
				return artifact.ErrInvalid
			}
			if e = application.DeleteArtifact(ctx, flags.Arg(2), version); e != nil {
				return e
			}
			return json.NewEncoder(out).Encode(map[string]string{"status": "deleted"})
		case "cleanup":
			report, e := application.CleanupArtifacts(ctx, flags.Arg(2) != "apply")
			if e != nil {
				return e
			}
			return json.NewEncoder(out).Encode(report)
		}
	}
	if skillCommand {
		raw, e := io.ReadAll(io.LimitReader(in, 16385))
		if e != nil || len(raw) > 16384 {
			return errors.New("invalid experience request")
		}
		q, ok := skill.ParseReview(raw)
		if !ok {
			return errors.New("invalid experience request")
		}
		result, e := application.ReviewExperience(ctx, q)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(result)
	}
	if memoryCommand {
		if flags.Arg(1) == "manage" {
			raw, e := io.ReadAll(io.LimitReader(in, 16385))
			if e != nil || len(raw) > 16384 {
				return errors.New("invalid memory request")
			}
			m, ok := memory.Parse(string(raw))
			if !ok {
				return errors.New("invalid memory request")
			}
			result, e := application.ManageMemory(ctx, flags.Arg(2), m)
			if e != nil {
				return e
			}
			return json.NewEncoder(out).Encode(result)
		}
		if flags.Arg(1) == "work" {
			if e := application.CurateMemory(ctx); e != nil {
				return e
			}
		}
		result, e := application.MemoryStatus(ctx)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(result)
	}
	if spendCommand {
		values, err := application.Spend(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(values)
	}
	if approvalCommand {
		switch flags.Arg(1) {
		case "list":
			values, err := application.Approvals(ctx)
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(values)
		case "reconcile":
			values, err := application.ReconcileTools(ctx)
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(values)
		default:
			decision := "approved"
			if flags.Arg(1) == "deny" {
				decision = "denied"
			}
			if err := application.DecideApproval(ctx, flags.Arg(2), flags.Arg(3), decision); err != nil {
				return err
			}
		}
		return json.NewEncoder(out).Encode(struct {
			Status string `json:"status"`
		}{"recorded"})
	}

	if terminalCommand {
		input, ok := in.(io.ReadCloser)
		if !ok {
			input = io.NopCloser(in)
		}
		if file, ok := in.(*os.File); ok {
			input, err = tui.OpenInput(file)
			if err != nil {
				return err
			}
		}
		return application.Terminal().Run(ctx, input, out)
	}
	if chatCommand {
		switch flags.Arg(1) {
		case "send", "send-parts":
			value, err := io.ReadAll(io.LimitReader(in, 16385))
			if err != nil || len(value) > 16384 {
				return errors.New("expected chat text of at most 16 KiB on stdin")
			}
			key := flags.Arg(2)
			if key == "" {
				key = uuid.Must(uuid.NewV7()).String()
			}
			var receipt agent.Receipt
			if flags.Arg(1) == "send-parts" {
				var parts []conversation.ContentPart
				dec := json.NewDecoder(bytes.NewReader(value))
				dec.DisallowUnknownFields()
				if dec.Decode(&parts) != nil {
					return conversation.ErrInvalid
				}
				var extra any
				if dec.Decode(&extra) != io.EOF {
					return conversation.ErrInvalid
				}
				receipt, err = application.SubmitParts(ctx, key, parts)
			} else {
				receipt, err = application.Actor(nil).Submit(ctx, key, string(value))
			}
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(receipt)
		case "work":
			workErr := application.Actor(nil).Drain(ctx)
			if ctx.Err() != nil && workErr == ctx.Err() {
				return nil
			}
			deliveryErr := application.Deliver(ctx)
			if err := errors.Join(workErr, deliveryErr); err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(struct {
				Status string `json:"status"`
			}{"drained"})
		case "history":
			messages, err := application.History(ctx)
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(messages)
		}
	}
	if modelCommand {
		switch flags.Arg(1) {
		case "accounts":
			accounts, err := application.Models().Accounts(ctx)
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(accounts)
		case "login-chatgpt":
			connection, err := application.Accounts().Login(ctx, flags.Arg(2), func(event llm.AuthEvent) { _ = json.NewEncoder(out).Encode(event) })
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(connection)
		case "refresh":
			connection, _, err := application.Accounts().Ensure(ctx, flags.Arg(2))
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(connection)
		case "logout":
			revoked, err := application.Accounts().Logout(ctx, flags.Arg(2))
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(map[string]any{"status": "logged_out", "remote_revocation_confirmed": revoked})
		case "account-catalog", "account-add":
			auth, _, err := application.Accounts().Ensure(ctx, flags.Arg(2))
			if err != nil {
				return err
			}
			models, err := application.AccountCatalog(ctx, auth.ConnectionID)
			if err != nil {
				return err
			}
			if flags.Arg(1) == "account-catalog" {
				return json.NewEncoder(out).Encode(models)
			}
			for _, m := range models {
				if m.ModelID == flags.Arg(3) {
					input := llm.ProfileInput{Name: m.ModelID, ProviderKind: "openai_account", WireAPI: "openai_responses", ModelID: m.ModelID, AuthConnectionID: auth.ConnectionID, ContextWindow: m.ContextWindow, MaxOutputTokens: min(m.MaxOutputTokens, m.ContextWindow), Compatibility: llm.Compatibility{ToolResultFormat: "openai"}, Capabilities: llm.Capabilities{TextInput: true}}
					profile, err := application.Models().AddProfile(ctx, input)
					if err != nil {
						return err
					}
					return json.NewEncoder(out).Encode(profile)
				}
			}
			return llm.ModelNotFound
		case "discover":
			models, err := application.Discover(ctx, flags.Arg(2), flags.Arg(3))
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(models)
		case "login":
			var connection llm.AuthConnection
			var err error
			if flags.NArg() == 3 {
				connection, err = application.LoginEnvironment(ctx, flags.Arg(2))
			} else {
				value, readErr := io.ReadAll(io.LimitReader(in, 8193))
				if readErr != nil || len(value) > 8192 {
					return llm.ErrSecretStore
				}
				value = []byte(strings.TrimSuffix(strings.TrimSuffix(string(value), "\n"), "\r"))
				connection, err = application.LoginKey(ctx, value)
				for i := range value {
					value[i] = 0
				}
			}
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(connection)
		case "catalog":
			var models []llm.CatalogModel
			var err error
			if flags.NArg() == 3 {
				models, err = application.CatalogProfile(ctx, flags.Arg(2))
			} else {
				models, err = application.Catalog(ctx)
			}
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(models)
		case "test":
			r, err := application.ModelControl().Test(ctx, flags.Arg(2))
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(r)
		case "select":
			if *testBridge == "" {
				fmt.Fprintln(errOut, "Privacy: model requests and tool schemas go only to the selected provider (OpenAI account requests use https://api.openai.com/v1) or your configured local endpoint. localhost resolves in the Go Host network namespace, not the Computer Guest. Credentials stay in SecretStore and transient Bridge memory. Erii never switches providers automatically.")
			}
			pv, err := strconv.ParseInt(flags.Arg(3), 10, 64)
			if err != nil || pv < 1 {
				return llm.ErrConflict
			}
			cv, err := strconv.ParseInt(flags.Arg(4), 10, 64)
			if err != nil || cv < 1 {
				return llm.ErrConflict
			}
			if err := application.ModelControl().Select(ctx, flags.Arg(2), pv, cv); err != nil {
				return err
			}
			d, err := application.Diagnostics(ctx)
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(d)
		case "cancel":
			d, err := application.Diagnostics(ctx)
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(d)
		}
		if flags.Arg(1) == "list" {
			profiles, err := application.Models().ListProfiles(ctx)
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(profiles)
		}
		profile, err := application.Models().AddProfile(ctx, input)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(profile)
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
