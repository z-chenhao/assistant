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
	"syscall"

	"github.com/z-chenhao/erii/internal/app"
	"github.com/z-chenhao/erii/internal/config"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/llm/fake"
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
	if len(args) == 2 && args[0] == "-test-provider-bridge" {
		return fake.Serve(in, out, args[1])
	}
	flags := flag.NewFlagSet("personal-agent", flag.ContinueOnError)
	flags.SetOutput(errOut)
	configPath := flags.String("config", "", "versioned YAML configuration file (optional)")
	testBridge := flags.String("test-bridge", "", "explicit deterministic test Bridge fixture; never a real provider")
	flags.Usage = func() {
		fmt.Fprintln(errOut, "Usage: personal-agent [-config path] [-test-bridge fixture] [status|relay|model add|model list|model catalog|model test ID|model select ID PROFILE_VERSION CONFIG_VERSION|model cancel]\nWithout a command, print startup diagnostics and wait for SIGINT/SIGTERM.\nrelay runs the internal event publisher until stopped.\nmodel add reads metadata JSON; test proves capabilities; select atomically activates a tested version. model cancel leaves setup unchanged.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	modelCommand := false
	if flags.Arg(0) == "model" {
		switch flags.Arg(1) {
		case "add", "list", "catalog", "cancel":
			modelCommand = flags.NArg() == 2
		case "test":
			modelCommand = flags.NArg() == 3
		case "select":
			modelCommand = flags.NArg() == 5
		}
	}
	if !modelCommand && (flags.NArg() > 1 || (flags.NArg() == 1 && flags.Arg(0) != "status" && flags.Arg(0) != "relay")) {
		return errors.New("expected status, relay, model add/list/catalog/test/select/cancel, or no command (see -help)")
	}
	var input llm.ProfileInput
	if modelCommand && flags.Arg(1) == "add" {
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
	var application *app.App
	if *testBridge != "" {
		application, err = app.OpenWithTestBridge(ctx, cfg, *testBridge)
	} else {
		application, err = app.Open(ctx, cfg)
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, application.Close()) }()
	if modelCommand {
		switch flags.Arg(1) {
		case "catalog":
			models, err := application.Catalog(ctx)
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
