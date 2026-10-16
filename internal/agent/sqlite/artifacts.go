package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"strings"
	"unicode/utf8"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/artifact"
	artifacttools "github.com/z-chenhao/erii/internal/artifact/tools"
	"github.com/z-chenhao/erii/internal/tool"
)

func runScope(ctx context.Context, tx *sql.Tx, id string) (artifact.Scope, error) {
	a := artifact.Scope{UserID: artifact.LocalUser, RunID: id}
	err := tx.QueryRowContext(ctx, `SELECT agent_id,coalesce(task_id,'') FROM run WHERE run_id=?`, id).Scan(&a.AgentID, &a.TaskID)
	return a, err
}
func artifactFailure(err error) tool.Result {
	switch {
	case errors.Is(err, artifact.ErrInvalid):
		return tool.Failure("INVALID_ARGUMENT")
	case errors.Is(err, artifact.ErrNotFound), errors.Is(err, artifact.ErrIntegrity):
		return tool.Failure("NOT_FOUND")
	case errors.Is(err, artifact.ErrLimit):
		return tool.Failure("RESOURCE_LIMIT")
	default:
		return tool.Failure("PROVIDER_UNAVAILABLE")
	}
}

// executeArtifact validates the accepted Host call; writes are deferred to
// storeOutput so metadata/publication and the Tool result commit atomically.
func (t *tools) executeArtifact(ctx context.Context, tx *sql.Tx, name, args string) (tool.Result, error) {
	a, err := runScope(ctx, tx, t.claim.RunID())
	if err != nil {
		return tool.Result{}, err
	}
	var v struct {
		Name      string   `json:"name"`
		MediaType string   `json:"mediaType"`
		Text      string   `json:"text"`
		Ref       string   `json:"artifactRef"`
		Refs      []string `json:"artifactRefs"`
		Offset    int64    `json:"offset"`
		Limit     int      `json:"limit"`
	}
	if json.Unmarshal([]byte(args), &v) != nil {
		return tool.Failure("INVALID_ARGUMENT"), nil
	}
	store := t.runs.host.Artifacts()
	switch name {
	case artifacttools.Save:
		return tool.Result{Output: &tool.BinaryOutput{Name: v.Name, MediaType: v.MediaType, Reader: strings.NewReader(v.Text)}}, nil
	case artifacttools.Publish:
		if v.Refs == nil {
			return tool.Failure("INVALID_ARGUMENT"), nil
		}
		return tool.Result{Text: "Final artifacts selected.", PublishRefs: v.Refs}, nil
	case artifacttools.Describe:
		d, e := store.DescribeTx(ctx, tx, a, v.Ref, true)
		if e != nil {
			return artifactFailure(e), nil
		}
		raw, e := encoded(d)
		return tool.Result{Text: raw}, e
	case artifacttools.List:
		list, e := store.ListTx(ctx, tx, a)
		if e != nil {
			return artifactFailure(e), nil
		}
		raw, e := encoded(list)
		return tool.Result{Text: raw}, e
	case artifacttools.Read:
		d, e := store.DescribeTx(ctx, tx, a, v.Ref, true)
		if e != nil {
			return artifactFailure(e), nil
		}
		// Binary media are passed by ref; never base64-encode them into history.
		media, _, _ := mime.ParseMediaType(d.MediaType)
		if !strings.HasPrefix(media, "text/") && media != "application/json" && !strings.HasSuffix(media, "+json") {
			return tool.Result{Text: "Binary content; use the artifact reference.", ArtifactRefs: []string{v.Ref}}, nil
		}
		f, e := store.OpenTx(ctx, tx, a, v.Ref)
		if e != nil {
			return artifactFailure(e), nil
		}
		defer f.Close()
		if v.Limit == 0 {
			v.Limit = 4096
		}
		if v.Limit < 1 || v.Limit > 4096 || v.Offset < 0 || v.Offset > d.ByteSize {
			return tool.Failure("INVALID_ARGUMENT"), nil
		}
		if _, e = io.CopyN(io.Discard, f, v.Offset); e != nil {
			return artifactFailure(e), nil
		}
		b, e := io.ReadAll(io.LimitReader(f, int64(v.Limit)))
		if e != nil {
			return artifactFailure(e), nil
		}
		// Byte ranges may cut a Unicode character. Reject that range explicitly.
		if !utf8.Valid(b) {
			return tool.Failure("INVALID_ARGUMENT"), nil
		}
		raw, e := encoded(struct {
			Text   string `json:"text"`
			Offset int64  `json:"offset"`
			Bytes  int    `json:"bytes"`
			Total  int64  `json:"total_bytes"`
		}{string(b), v.Offset, len(b), d.ByteSize})
		return tool.Result{Text: raw}, e
	}
	return tool.Failure("NOT_FOUND"), nil
}
func (t *tools) storeOutput(ctx context.Context, tx *sql.Tx, id string, result tool.Result) (tool.Result, error) {
	a, err := runScope(ctx, tx, t.claim.RunID())
	if err != nil {
		return tool.Result{}, err
	}
	store := t.runs.host.Artifacts()
	var policy, name string
	if err = tx.QueryRowContext(ctx, `SELECT policy_json,tool_name FROM tool_invocation WHERE invocation_id=? AND run_id=?`, id, a.RunID).Scan(&policy, &name); err != nil {
		if err == sql.ErrNoRows {
			return tool.Result{}, agent.ErrConflict
		}
		return tool.Result{}, err
	}
	if result.PublishRefs != nil && name != artifacttools.Publish {
		return tool.Failure("PERMISSION_DENIED"), nil
	}
	var p tool.Policy
	if json.Unmarshal([]byte(policy), &p) != nil {
		return tool.Result{}, artifact.ErrStorage
	}
	if p.OutputBytes == 0 {
		p.OutputBytes = 16384
	}
	if result.Error == nil && (result.Output != nil || len(result.Text) > p.OutputBytes) {
		output := result.Output
		if output == nil {
			output = &tool.BinaryOutput{Name: "tool-output.txt", MediaType: "text/plain", Reader: strings.NewReader(result.Text)}
		}
		ref, e := store.PutTx(ctx, tx, a, artifact.Input{Name: output.Name, MediaType: output.MediaType}, output.Reader, "tool:"+id)
		if e != nil {
			if errors.Is(e, artifact.ErrLimit) || errors.Is(e, artifact.ErrInvalid) {
				return artifactFailure(e), nil
			}
			return tool.Result{}, e
		}
		result.Text = "Saved artifact."
		result.ArtifactRefs = append(result.ArtifactRefs, ref)
		result.Output = nil
	}
	if result.Error != nil {
		return tool.Normalize(result), nil
	}
	for _, ref := range result.ArtifactRefs {
		if err = store.PinTx(ctx, tx, a, ref, "tool", id); err != nil {
			return artifactFailure(err), nil
		}
	}
	if result.PublishRefs != nil {
		if err = store.PublishTx(ctx, tx, a, result.PublishRefs); err != nil {
			if errors.Is(err, artifact.ErrNotFound) || errors.Is(err, artifact.ErrInvalid) || errors.Is(err, artifact.ErrIntegrity) {
				return artifactFailure(err), nil
			}
			return tool.Result{}, err
		}
		result.ArtifactRefs = append(result.ArtifactRefs, result.PublishRefs...)
		result.PublishRefs = nil
	}
	return result, nil
}
func published(ctx context.Context, tx *sql.Tx, runID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT artifact_ref FROM artifact_publication WHERE run_id=? ORDER BY artifact_ref`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	refs := []string{}
	for rows.Next() {
		var ref string
		if err = rows.Scan(&ref); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}
