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

	"github.com/mattn/go-isatty"
	"github.com/z-chenhao/collie/internal/app"
	"github.com/z-chenhao/collie/internal/computer/macosvm"
	"github.com/z-chenhao/collie/internal/config"
	"github.com/z-chenhao/collie/internal/startup"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	args := os.Args[1:]
	if len(args) == 0 && isatty.IsTerminal(os.Stdin.Fd()) {
		args = []string{"start"}
	}
	if err := run(ctx, args, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "collie:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) (err error) {
	if handled, err := runTestPeer(ctx, args, in, out); handled {
		return err
	}
	options, err := parseOptions(args, errOut)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	flags := options.flags
	command, err := selectCommand(flags, in)
	if err != nil {
		return err
	}
	input, err := readModelInput(command, flags, in)
	if err != nil {
		return err
	}
	cfg, err := config.Load(options.configPath, os.LookupEnv)
	if err != nil {
		return err
	}
	// Bundled defaults belong to the user startup/diagnostic path. Execution
	// commands still require their explicit configuration; never attach a new
	// provider bridge while reclaiming an existing Run.
	if (command.start || command.kind == commandDoctor || flags.Arg(0) == "status") && options.configPath == "" && options.testBridge == "" {
		home, err := runtimeHome()
		if err != nil {
			return err
		}
		configureRuntime(&cfg, home)
	}
	if command.kind == commandDoctor {
		return doctor(ctx, cfg, out)
	}
	if command.start {
		if options.configPath == "" && options.testBridge == "" {
			var diagnostics bytes.Buffer
			if err := doctor(ctx, cfg, &diagnostics); err != nil {
				fmt.Fprint(errOut, diagnostics.String())
				return err
			}
		}
		if _, external := os.LookupEnv("COLLIE_NATS_URL"); options.configPath == "" && !external {
			broker, err := startup.OpenLocalBroker(ctx, cfg.App.DataRoot)
			if err != nil {
				return err
			}
			defer broker.Close()
			cfg.NATS.URL = broker.URL()
		}
	}
	if handled, e := runOfflineOperation(ctx, cfg, flags, options.testSecrets, out); handled {
		return e
	}
	if command.kind == commandComputer && flags.Arg(1) == "prepare-base" {
		if cfg.Computer == nil {
			return errors.New("computer configuration required")
		}
		if err := macosvm.ProvisionBase(ctx, cfg.Computer.Root, flags.Arg(2), cfg.Computer.Compatibility); err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(map[string]string{"base": "sealed"})
	}
	var application *app.App
	if (options.testBridge == "" || options.testEvents) && (command.kind == commandWeb || command.kind == commandTerminal || command.kind == commandAutomation && flags.Arg(1) == "work") {
		application, err = app.OpenRuntime(ctx, cfg, options.testBridge, options.testSecrets)
	} else if command.kind == commandModel || command.kind == commandOps || flags.Arg(0) == "status" {
		application, err = app.OpenControl(ctx, cfg, options.testBridge, options.testSecrets)
	} else if options.testBridge != "" {
		application, err = app.OpenWithTestBridge(ctx, cfg, options.testBridge)
	} else if options.testSecrets != "" {
		application, err = app.OpenWithTestSecrets(ctx, cfg, options.testSecrets)
	} else {
		application, err = app.Open(ctx, cfg)
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, application.Close()) }()
	if command.kind == commandOps {
		return runOperations(ctx, application, cfg, flags, out)
	}
	if err := options.configureClock(application); err != nil {
		return err
	}
	if command.kind == commandComputer {
		state, err := application.ComputerStatus(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(state)
	}
	if err := options.configureExecution(ctx, application, out); err != nil {
		return err
	}
	return dispatchCommand(ctx, application, cfg, command, options, input, in, out, errOut)
}
