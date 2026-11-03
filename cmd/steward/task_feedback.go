package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"

	"github.com/z-chenhao/assistant/internal/app"
)

func runTask(ctx context.Context, application *app.App, flags *flag.FlagSet, in io.Reader, out io.Writer) error {
	if flags.Arg(1) == "feedbacks" {
		items, err := application.TaskFeedbacks(ctx, flags.Arg(2), flags.Arg(3))
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(items)
	}
	answer, err := io.ReadAll(io.LimitReader(in, (16<<10)+1))
	if err != nil || len(answer) > 16<<10 {
		return errors.New("expected feedback text of at most 16 KiB on stdin")
	}
	receipt, err := application.RespondTaskFeedback(ctx, flags.Arg(2), string(answer))
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(receipt)
}
