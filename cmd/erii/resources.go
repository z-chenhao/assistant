package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"strconv"

	"github.com/z-chenhao/erii/internal/app"
	"github.com/z-chenhao/erii/internal/artifact"
	"github.com/z-chenhao/erii/internal/memory"
	"github.com/z-chenhao/erii/internal/skill"
)

func runArtifact(ctx context.Context, application *app.App, flags *flag.FlagSet, in io.Reader, out io.Writer) error {
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
	return nil
}

func runSkill(ctx context.Context, application *app.App, flags *flag.FlagSet, in io.Reader, out io.Writer) error {
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

func runMemory(ctx context.Context, application *app.App, flags *flag.FlagSet, in io.Reader, out io.Writer) error {
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

func runApproval(ctx context.Context, application *app.App, flags *flag.FlagSet, in io.Reader, out io.Writer) error {
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
