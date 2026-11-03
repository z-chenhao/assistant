package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"strconv"

	"github.com/z-chenhao/assistant/internal/app"
	"github.com/z-chenhao/assistant/internal/observer/github"
)

func runGitHub(ctx context.Context, application *app.App, flags *flag.FlagSet, in io.Reader, out io.Writer) error {
	switch flags.Arg(1) {
	case "bind", "unbind":
		raw, err := io.ReadAll(io.LimitReader(in, 4097))
		if err != nil || len(raw) > 4096 {
			return errors.New("invalid GitHub task binding request")
		}
		q, err := github.ParseBinding(string(raw), flags.Arg(1) == "bind")
		if err != nil {
			return err
		}
		binding, err := application.BindGitHubTask(ctx, q, flags.Arg(1) == "bind")
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(binding)
	case "binding":
		binding, err := application.GitHubTaskBinding(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(binding)
	case "poll":
		if err := application.PollGitHub(ctx); err != nil {
			return err
		}
	case "stop", "resume":
		if err := application.SetGitHubWatch(ctx, flags.Arg(1) == "resume"); err != nil {
			return err
		}
	case "events", "decisions":
		var before int64
		if flags.NArg() == 3 {
			var err error
			before, err = strconv.ParseInt(flags.Arg(2), 10, 64)
			if err != nil || before < 1 {
				return errors.New("invalid GitHub observation cursor")
			}
		}
		if flags.Arg(1) == "decisions" {
			decisions, err := application.GitHubRuleDecisions(ctx, before)
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(decisions)
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
