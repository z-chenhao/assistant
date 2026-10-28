package sqlite

import (
	"context"
	"database/sql"
	"io"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/artifact"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/media"
)

// Only the accepted generation's explicit selections can resolve pixels. The
// scoped read verifies integrity outside the transaction, then checks the same
// claim/privacy fence again. Bytes exist only for dispatch, never in requests.
func (r *Runs) resolveGenerationImages(c agent.RunClaim, request *llm.GenerationRequest) {
	if !request.Snapshot.Capabilities.ImageInput {
		return
	}
	selected := map[string]bool{}
	for _, message := range request.Messages {
		for _, ref := range message.ImageRefs {
			selected[ref] = true
		}
	}
	if len(selected) == 0 {
		return
	}
	request.ResolveImage = func(ctx context.Context, ref string) (string, []byte, error) {
		if !selected[ref] {
			return "", nil, artifact.ErrNotFound
		}
		return r.readImage(ctx, ref, func(tx *sql.Tx) (artifact.Scope, error) {
			if err := fence(ctx, tx, c); err != nil {
				return artifact.Scope{}, err
			}
			return runScope(ctx, tx, c.RunID())
		})
	}
}

// Visual dispatch and OCR share the scoped, integrity-checked source read.
// Callers supply their generation or Tool claim; neither accepts Host paths.
func (r *Runs) readImage(ctx context.Context, ref string, authorize func(*sql.Tx) (artifact.Scope, error)) (string, []byte, error) {
	file, descriptor, err := r.openArtifact(ctx, ref, authorize)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()
	if descriptor.ByteSize > media.MaxImageBytes {
		return "", nil, artifact.ErrLimit
	}
	data, err := io.ReadAll(io.LimitReader(file, media.MaxImageBytes+1))
	if err == nil && len(data) > media.MaxImageBytes {
		err = artifact.ErrLimit
	}
	return descriptor.MediaType, data, err
}
