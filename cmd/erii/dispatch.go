package main

import (
	"context"
	"encoding/json"
	"io"

	"github.com/z-chenhao/erii/internal/app"
	"github.com/z-chenhao/erii/internal/config"
	"github.com/z-chenhao/erii/internal/llm"
)

func dispatchCommand(ctx context.Context, application *app.App, cfg config.Config, command command, options commandOptions, input llm.ProfileInput, in io.Reader, out, errOut io.Writer) error {
	flags := options.flags
	foreground := func(action func(context.Context) error) error {
		if options.testBridge == "" || options.testEvents {
			return application.WithEvents(ctx, cfg, action)
		}
		application.StartMemory(ctx)
		return action(ctx)
	}
	if command.kind != commandTerminal && flags.NArg() == 0 || command.kind == commandChat && flags.Arg(1) == "work" {
		application.StartMemory(ctx)
	}
	switch command.kind {
	case commandWeb:
		return runWeb(ctx, application, cfg, out, foreground)
	case commandChannel:
		return runChannel(ctx, application, flags, in, out)
	case commandDelivery:
		return runDelivery(ctx, application, flags, in, out)
	case commandAutomation:
		return runAutomation(ctx, application, flags, in, out, foreground)
	case commandArtifact:
		return runArtifact(ctx, application, flags, in, out)
	case commandSkill:
		return runSkill(ctx, application, flags, in, out)
	case commandMemory:
		return runMemory(ctx, application, flags, in, out)
	case commandSpend:
		values, err := application.Spend(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(values)
	case commandApproval:
		return runApproval(ctx, application, flags, in, out)

	case commandTerminal:
		return runTerminal(ctx, application, in, out, foreground)
	case commandChat:
		return runChat(ctx, application, flags, in, out)
	case commandModel:
		return runModel(ctx, application, flags, in, out, input, options.testBridge != "", errOut)
	}

	diagnostics, err := application.Diagnostics(ctx)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(out).Encode(diagnostics); err != nil {
		return err
	}
	if command.kind == commandRelay {
		return application.Relay(ctx, cfg)
	}
	if flags.NArg() == 0 {
		<-ctx.Done()
	}
	return nil
}
