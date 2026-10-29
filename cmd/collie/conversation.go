package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"

	"github.com/google/uuid"
	"github.com/z-chenhao/collie/internal/agent"
	"github.com/z-chenhao/collie/internal/app"
	"github.com/z-chenhao/collie/internal/conversation"
	"github.com/z-chenhao/collie/internal/tui"
)

func runChat(ctx context.Context, application *app.App, flags *flag.FlagSet, in io.Reader, out io.Writer) error {
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
	return nil
}

func runTerminal(ctx context.Context, application *app.App, in io.Reader, out io.Writer, foreground func(func(context.Context) error) error) error {
	var err error
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
	return foreground(func(ctx context.Context) error { return application.Terminal().Run(ctx, input, out) })
}
