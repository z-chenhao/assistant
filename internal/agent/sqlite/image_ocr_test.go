package sqlite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/z-chenhao/collie/internal/agent"
	"github.com/z-chenhao/collie/internal/artifact"
	"github.com/z-chenhao/collie/internal/conversation"
	"github.com/z-chenhao/collie/internal/llm"
	"github.com/z-chenhao/collie/internal/media"
	"github.com/z-chenhao/collie/internal/media/ocr"
	"github.com/z-chenhao/collie/internal/tool"
	"github.com/z-chenhao/collie/internal/tool/codemode"
)

type imageOCRFixture struct {
	calls     atomic.Int32
	err       error
	hash      string
	block     bool
	timeoutMS int
}

func (f *imageOCRFixture) ResourceHash() string { return f.hash }
func (f *imageOCRFixture) Extract(ctx context.Context, kind string, data []byte) (ocr.Document, error) {
	f.calls.Add(1)
	if f.block {
		<-ctx.Done()
		return ocr.Document{}, ctx.Err()
	}
	if _, err := media.DecodeImage(kind, data); err != nil {
		return ocr.Document{}, err
	}
	if f.err != nil {
		return ocr.Document{}, f.err
	}
	d := ocr.Document{Method: "ocr", Languages: []string{"eng"}, Width: 640, Height: 120, Text: strings.Repeat("OCR text ", 500)}
	for range 20 {
		d.Words = append(d.Words, ocr.Word{Text: "OCR", X: 1, Y: 1, Width: 10, Height: 10, Confidence: 95})
	}
	return d, nil
}

func admitImageOCR(t *testing.T, engine *imageOCRFixture, granted, code bool) (*Store, *Runs, agent.RunClaim, tool.Executor, string, []llm.ToolCall, string) {
	t.Helper()
	ctx := context.Background()
	s, r, _, a, p := loopFixture(t)
	definition := ocr.Definition(engine.ResourceHash())
	if engine.timeoutMS > 0 {
		definition.Spec.TimeoutMS = engine.timeoutMS
	}
	defs := []tool.Definition{definition}
	if code {
		defs = append(defs, codemode.Definition())
		r.ConfigureJavaScript(realCodeRuntime(t))
	}
	registry, err := tool.NewRegistry(defs)
	if err != nil {
		t.Fatal(err)
	}
	r.registry = registry
	a.Registry = registry
	r.ConfigureImageOCR(engine)
	pixels, err := os.ReadFile("../../media/ocr/testdata/text.png")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := s.Artifacts().Put(ctx, artifactOwner(t, s), artifact.Input{Name: "scan.png", MediaType: "image/png"}, bytes.NewReader(pixels))
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]string{"artifactRef": ref})
	calls := []llm.ToolCall{{ID: "ocr", Name: ocr.Name, Arguments: string(args)}}
	if code {
		source := "const r = await tools.builtin__image_ocr(" + string(args) + "); text(r);"
		args, _ = json.Marshal(map[string]string{"source": source})
		calls = []llm.ToolCall{{ID: "code-ocr", Name: codemode.ToolName, Arguments: string(args)}}
	}
	p.respond = func(llm.GenerationRequest) (llm.GenerationResult, error) {
		return llm.GenerationResult{ToolCalls: calls}, nil
	}
	parts := conversation.Text("Read the image text")
	if granted {
		parts = append(parts, conversation.ContentPart{Type: "image", ArtifactRef: ref})
	}
	receipt, err := r.Submit(ctx, "ocr", parts)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := r.Claim(ctx, receipt.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Initialize(ctx, claim); err != nil {
		t.Fatal(err)
	}
	run, err := r.Load(ctx, claim)
	if err != nil {
		t.Fatal(err)
	}
	gen, err := r.Generation(ctx, claim, run)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Generate(ctx, gen.Request, nil); err != nil {
		t.Fatal(err)
	}
	return s, r, claim, tool.Executor{Registry: registry, Store: r.ToolStore(claim), Concurrency: 1}, gen.Request.GenerationID, calls, ref
}

func TestImageOCRSharedDirectCodeModeAndDurableReplay(t *testing.T) {
	for _, code := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "code"}[code], func(t *testing.T) {
			ctx := context.Background()
			engine := &imageOCRFixture{hash: "ocr-fixture-v1"}
			s, r, claim, e, exchange, calls, ref := admitImageOCR(t, engine, true, code)
			out, err := e.Execute(ctx, exchange, calls)
			if err != nil || len(out) != 1 || out[0].Error != nil {
				t.Fatal(out, err)
			}
			var raw string
			if err = s.db.QueryRow(`SELECT result_json FROM tool_invocation WHERE tool_name=?`, ocr.Name).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var result tool.Result
			var preview ocr.Document
			if err = json.Unmarshal([]byte(raw), &result); err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal([]byte(result.Text), &preview); err != nil {
				t.Fatal(err)
			}
			if preview.SourceRef != ref || preview.DocumentRef == "" || !preview.Truncated || len(preview.Words) != 16 || len(result.ArtifactRefs) != 2 || len(result.ImageRefs) != 0 {
				t.Fatal("wrong OCR envelope", result)
			}
			scope := artifact.Scope{UserID: artifact.LocalUser, AgentID: idAgent(t, s), RunID: claim.RunID()}
			var full ocr.Document
			if err = json.Unmarshal([]byte(readArtifact(t, s, scope, preview.DocumentRef)), &full); err != nil {
				t.Fatal(err)
			}
			if full.SourceRef != ref || len(full.Text) <= 2048 || len(full.Words) != 20 || full.Truncated {
				t.Fatal("full OCR artifact lost", full)
			}
			if scalar(t, s, `SELECT count(*) FROM artifact_publication`) != 0 || scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE tool_name=?`, ocr.Name) != 1 {
				t.Fatal("OCR auto-published or bypassed invocation")
			}
			if err = r.Release(ctx, claim); err != nil {
				t.Fatal(err)
			}
			recovered, err := r.Claim(ctx, claim.RunID())
			if err != nil {
				t.Fatal(err)
			}
			e.Store = r.ToolStore(recovered)
			r.ConfigureImageOCR(nil) // committed replay is independent of the engine
			again, err := e.Execute(ctx, exchange, calls)
			if err != nil || !bytes.Equal(mustJSON(t, out), mustJSON(t, again)) || engine.calls.Load() != 1 {
				t.Fatal("OCR replay changed or executed twice", err, engine.calls.Load())
			}
		})
	}
}

func TestImageOCRScopeAndNormalizedFailures(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		err        error
		granted    bool
	}{
		{"ungranted", "NOT_FOUND", nil, false},
		{"invalid", "INVALID_ARGUMENT", media.ErrInvalid, true},
		{"oversized", "RESOURCE_LIMIT", media.ErrLimit, true},
		{"unavailable", "PROVIDER_UNAVAILABLE", ocr.ErrUnavailable, true},
		{"resources_changed", "PERMISSION_DENIED", ocr.ErrChanged, true},
		{"timeout", "TIMEOUT", context.DeadlineExceeded, true},
		{"cancelled", "CANCELLED", context.Canceled, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &imageOCRFixture{hash: "ocr-fixture-v1", err: tc.err}
			s, _, _, e, exchange, calls, _ := admitImageOCR(t, engine, tc.granted, false)
			out, err := e.Execute(context.Background(), exchange, calls)
			if err != nil || len(out) != 1 || out[0].Error == nil || out[0].Error.Code != tc.code {
				t.Fatal(out, err)
			}
			if !tc.granted && engine.calls.Load() != 0 {
				t.Fatal("ungranted pixels reached OCR")
			}
			if scalar(t, s, `SELECT count(*) FROM artifact`) != 1 {
				t.Fatal("failure saved output")
			}
		})
	}
}

func TestImageOCRCannotSubstituteFrozenResourcesOrInvocation(t *testing.T) {
	ctx := context.Background()
	engine := &imageOCRFixture{hash: "ocr-fixture-v1"}
	_, r, _, e, exchange, calls, _ := admitImageOCR(t, engine, true, false)
	invs, err := e.Store.Accept(ctx, exchange, calls, e.Registry.Describe())
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Store.Start(ctx, invs[0].ID); err != nil {
		t.Fatal(err)
	}
	forged := invs[0]
	forged.Call.Name = "builtin__read"
	forged.Call.Arguments = `{"path":"/etc/passwd"}`
	result, err := e.Store.(tool.HostStore).ExecuteBuiltin(ctx, forged)
	if err != nil || result.Error != nil || engine.calls.Load() != 1 {
		t.Fatal("durable OCR invocation was redirected", result, err)
	}
	engine.hash = "changed"
	result, err = e.Store.(tool.HostStore).ExecuteBuiltin(ctx, forged)
	if err != nil || result.Error == nil || result.Error.Code != "PERMISSION_DENIED" || engine.calls.Load() != 1 {
		t.Fatal("resource substitution admitted", result, err)
	}
	updated, _ := tool.NewRegistry([]tool.Definition{ocr.Definition(engine.ResourceHash())})
	if _, err = updated.Select(r.registry.Describe()); !errors.Is(err, llm.ModelIncompatible) {
		t.Fatal("frozen capability changed", err)
	}
	if len(agent.WorkerTools(e.Registry.Describe())) != 1 || len(agent.ChildTools(e.Registry.Describe())) != 1 {
		t.Fatal("read-only OCR missing from Worker roles")
	}
}

func TestImageOCRDeadlineCommitsKnownFailureWithoutArtifact(t *testing.T) {
	engine := &imageOCRFixture{hash: "ocr-fixture-v1", block: true, timeoutMS: 30}
	s, _, _, e, exchange, calls, _ := admitImageOCR(t, engine, true, false)
	out, err := e.Execute(context.Background(), exchange, calls)
	if err != nil || len(out) != 1 || out[0].Error == nil || out[0].Error.Code != "TIMEOUT" {
		t.Fatal(out, err)
	}
	if scalar(t, s, `SELECT count(*) FROM artifact`) != 1 || scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE status='failed'`) != 1 {
		t.Fatal("deadline leaked output or left invocation running")
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
