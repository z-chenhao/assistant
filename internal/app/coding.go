package app

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/z-chenhao/assistant/internal/artifact"
	"github.com/z-chenhao/assistant/internal/coding"
)

func (a *App) CodeReport(ctx context.Context, ref string, withDiff bool) (coding.Report, error) {
	return a.store.CodeReport(ctx, ref, withDiff)
}

func (a *App) SaveCodeReport(ctx context.Context, ref string) (artifact.Descriptor, error) {
	report, err := a.store.CodeReport(ctx, ref, true)
	if err != nil {
		return artifact.Descriptor{}, err
	}
	// Paths are an owner inspect detail, not part of a model-visible artifact.
	report.Directory = ""
	raw, err := json.Marshal(report)
	if err != nil {
		return artifact.Descriptor{}, err
	}
	return a.SaveArtifact(ctx, artifact.Input{Name: "code-review.json", MediaType: "application/json"}, bytes.NewReader(raw))
}
