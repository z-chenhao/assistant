package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"strconv"

	"github.com/z-chenhao/assistant/internal/app"
)

func runGitHub(ctx context.Context, application *app.App, flags *flag.FlagSet, out io.Writer) error {
	switch flags.Arg(1) {
	case "poll":
		if err := application.PollGitHub(ctx); err != nil {
			return err
		}
	case "stop", "resume":
		if err := application.SetGitHubWatch(ctx, flags.Arg(1) == "resume"); err != nil {
			return err
		}
	case "events":
		var before int64
		if flags.NArg() == 3 {
			var err error
			before, err = strconv.ParseInt(flags.Arg(2), 10, 64)
			if err != nil || before < 1 {
				return errors.New("invalid GitHub observation cursor")
			}
		}
		events, err := application.GitHubObservations(ctx, before)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(events)
	}
	status, err := application.GitHubStatus(ctx)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(status)
}
