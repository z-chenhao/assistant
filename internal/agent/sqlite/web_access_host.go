package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"io"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/artifact"
	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/plugin/webaccess"
	"github.com/z-chenhao/assistant/internal/tool"
)

type webAccessHost struct {
	tools      *tools
	invocation tool.Invocation
	run        agent.Run
}

func (h webAccessHost) Generate(ctx context.Context, key string, call webaccess.ModelCall) (llm.GenerationResult, error) {
	return h.tools.webModelGeneration(ctx, h.invocation, h.run, key, call)
}
func (h webAccessHost) ReadEvidence(ctx context.Context, ref string) ([]byte, error) {
	t := h.tools
	file, _, e := t.runs.openArtifact(ctx, ref, func(tx *sql.Tx) (scope artifact.Scope, err error) {
		if err = t.fence(ctx, tx); err != nil {
			return
		}
		return runScope(ctx, tx, t.claim.RunID())
	})
	if e != nil {
		return nil, errors.New(artifactFailure(e).Error.Code)
	}
	defer file.Close()
	raw, e := io.ReadAll(io.LimitReader(file, webaccess.MaxEvidenceBytes+1))
	if e != nil || len(raw) > webaccess.MaxEvidenceBytes {
		return nil, errors.New("INVALID_ARGUMENT")
	}
	return raw, nil
}
