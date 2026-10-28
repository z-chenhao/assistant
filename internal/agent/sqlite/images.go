package sqlite

import (
	"context"
	"database/sql"
	"io"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/artifact"
	"github.com/z-chenhao/erii/internal/llm"
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
		file, descriptor, err := r.openArtifact(ctx, ref, func(tx *sql.Tx) (artifact.Scope, error) {
			if err := fence(ctx, tx, c); err != nil {
				return artifact.Scope{}, err
			}
			return runScope(ctx, tx, c.RunID())
		})
		if err != nil {
			return "", nil, err
		}
		defer file.Close()
		if descriptor.ByteSize > llm.MaxInputImageBytes {
			return "", nil, artifact.ErrLimit
		}
		data, err := io.ReadAll(io.LimitReader(file, llm.MaxInputImageBytes+1))
		if err == nil && len(data) > llm.MaxInputImageBytes {
			err = artifact.ErrLimit
		}
		return descriptor.MediaType, data, err
	}
}
