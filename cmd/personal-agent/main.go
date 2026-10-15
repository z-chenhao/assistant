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
	"syscall"

	"github.com/z-chenhao/erii/internal/app"
	"github.com/z-chenhao/erii/internal/config"
	"github.com/z-chenhao/erii/internal/llm"
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
	flags := flag.NewFlagSet("personal-agent", flag.ContinueOnError)
	flags.SetOutput(errOut)
	configPath := flags.String("config", "", "versioned YAML configuration file (optional)")
	flags.Usage = func() {
		fmt.Fprintln(errOut, "Usage: personal-agent [-config path] [status|relay|model add|model list]\nWithout a command, print startup diagnostics and wait for SIGINT/SIGTERM.\nrelay runs the internal event publisher until stopped.\nmodel add reads one metadata-only JSON object from stdin; all profiles remain unverified.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	modelCommand := flags.NArg() == 2 && flags.Arg(0) == "model" && (flags.Arg(1) == "add" || flags.Arg(1) == "list")
	if !modelCommand && (flags.NArg() > 1 || (flags.NArg() == 1 && flags.Arg(0) != "status" && flags.Arg(0) != "relay")) {
		return errors.New("expected status, relay, model add, model list, or no command (see -help)")
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
	application, err := app.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, application.Close()) }()
	if modelCommand {
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
