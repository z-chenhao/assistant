package main

import (
	"context"
	"encoding/json"
	"io"

	"github.com/z-chenhao/assistant/internal/app"
	"github.com/z-chenhao/assistant/internal/evidence"
)

func runEvidence(ctx context.Context, application *app.App, in io.Reader, out io.Writer) error {
	raw, err := io.ReadAll(io.LimitReader(in, 4097))
	if err != nil || len(raw) > 4096 {
		return evidence.ErrInvalid
	}
	q, err := evidence.ParseOwner(string(raw))
	if err != nil {
		return err
	}
	page, err := application.ReadEvidence(ctx, q)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(page)
}
