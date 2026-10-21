package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ProbeStepTimeout allows cold local models to load and think. Each check gets
// its own bound; expected cancellation does not cancel subsequent checks.
const ProbeStepTimeout = 2 * time.Minute

type ProbeStage int

const (
	ProbeConnection ProbeStage = iota
	ProbeTools
	ProbeContinuation
	ProbeCancellation
	ProbeClipping
	ProbeImage
)

func (s ProbeStage) String() string {
	switch s {
	case ProbeConnection:
		return "text and streaming"
	case ProbeTools:
		return "Tool calls"
	case ProbeContinuation:
		return "Tool result continuation"
	case ProbeCancellation:
		return "cancellation"
	case ProbeClipping:
		return "output clipping"
	case ProbeImage:
		return "image input"
	default:
		return "capability check"
	}
}

// ProbeError adds safe, Host-owned diagnostics without exposing provider bodies.
// Unwrap preserves existing invalidation and normalized provider error checks.
type ProbeError struct {
	Stage   ProbeStage
	Timeout time.Duration
	Cause   error
}

func (e *ProbeError) Error() string {
	if e.Timeout > 0 {
		return fmt.Sprintf("model probe %s: timed out after %s", e.Stage, e.Timeout)
	}
	code := InvalidProviderResponse
	for _, known := range []ProviderError{AuthRequired, ModelNotFound, ModelIncompatible, RateLimited, ContextLengthExceeded, ProviderUnavailable, GenerationCancelled, InvalidProviderResponse} {
		if errors.Is(e.Cause, known) {
			code = known
			break
		}
	}
	return fmt.Sprintf("model probe %s: %s", e.Stage, code)
}
func (e *ProbeError) Unwrap() error { return e.Cause }

type Prober struct {
	Provider    LLMProvider
	stepTimeout time.Duration // internal override for deterministic deadline tests
}

// Run verifies streaming, a Tool Call/Result round trip, cancellation, and Host
// output bounds using the same LLMProvider boundary as generation.
func (p Prober) Run(ctx context.Context, profile Profile) (ProbeResult, error) {
	return p.RunWithProgress(ctx, profile, nil)
}

// RunWithProgress reports only fixed stage labels to user control surfaces.
func (p Prober) RunWithProgress(parent context.Context, profile Profile, progress func(ProbeStage)) (result ProbeResult, err error) {
	limit := p.stepTimeout
	if limit <= 0 {
		limit = ProbeStepTimeout
	}
	var ctx context.Context
	var cancel context.CancelFunc
	var stage ProbeStage
	start := func(next ProbeStage) {
		if cancel != nil {
			cancel()
		}
		stage = next
		ctx, cancel = context.WithTimeout(parent, limit)
		if progress != nil {
			progress(stage)
		}
	}
	start(ProbeConnection)
	defer func() {
		if err != nil {
			detail := &ProbeError{Stage: stage, Cause: err}
			if errors.Is(ctx.Err(), context.DeadlineExceeded) && parent.Err() == nil {
				detail.Timeout = limit
				detail.Cause = errors.Join(err, context.DeadlineExceeded)
			}
			err = detail
		}
		cancel()
	}()
	var ids [8]string
	for i := range ids {
		value, err := uuid.NewV7()
		if err != nil {
			return ProbeResult{}, ProviderUnavailable
		}
		ids[i] = value.String()
	}
	next := 0
	id := func() string { value := ids[next]; next++; return value }
	hash, err := EndpointHash(profile.Endpoint)
	if err != nil {
		return ProbeResult{}, err
	}
	s := Snapshot{SnapshotID: id(), ProfileID: profile.ProfileID, ProfileVersion: profile.Version,
		ProviderKind: profile.ProviderKind, EndpointHash: hash, WireAPI: profile.WireAPI, ModelID: profile.ModelID,
		AuthConnectionID: profile.AuthConnectionID, ContextWindow: profile.ContextWindow, MaxOutputTokens: profile.MaxOutputTokens,
		Compatibility: Compatibility{ToolResultFormat: "openai"}, Capabilities: Capabilities{TextInput: true}}
	run := id()
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer stop()
		_ = p.Provider.Dispose(cleanup, run)
	}()
	request := func(text string) GenerationRequest {
		return GenerationRequest{GenerationID: id(), RunID: run, SessionID: run, Snapshot: s, Messages: []Message{{Role: "user", Text: text}}}
	}
	connection, err := p.Provider.Generate(ctx, request("Reply with OK."), func(e GenerationEvent) { result.Streaming = result.Streaming || e.Kind == "text_delta" })
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(connection.Text) == "" || !result.Streaming {
		return result, ModelIncompatible
	}
	start(ProbeTools)
	r := request("Call probe_echo with value OK. After its result, reply exactly with the tool result text.")
	r.Tools = []ToolSpec{{Name: "probe_echo", InputSchema: `{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}`}}
	called, err := p.Provider.Generate(ctx, r, nil)
	if err != nil {
		return result, err
	}
	if len(called.ToolCalls) != 1 || called.ToolCalls[0].Name != "probe_echo" || called.ToolCalls[0].ID == "" {
		return result, ModelIncompatible
	}
	var arguments struct {
		Value string `json:"value"`
	}
	if json.Unmarshal([]byte(called.ToolCalls[0].Arguments), &arguments) != nil || arguments.Value != "OK" {
		return result, ModelIncompatible
	}
	result.ToolCalls = true
	start(ProbeContinuation)
	r.GenerationID = id()
	r.Messages = append(r.Messages, Message{Role: "assistant", ToolCalls: called.ToolCalls}, Message{Role: "tool", Text: "PROBE_RESULT_OK", ToolCallID: called.ToolCalls[0].ID})
	continued, err := p.Provider.Continue(ctx, r, nil)
	if err != nil {
		return result, err
	}
	result.Continuation = continued.Text == "PROBE_RESULT_OK"
	if !result.Continuation {
		return result, ModelIncompatible
	}
	start(ProbeCancellation)
	cancelCtx, stop := context.WithCancel(ctx)
	_, err = p.Provider.Generate(cancelCtx, request("Stream until cancelled."), func(e GenerationEvent) {
		if e.Kind == "text_delta" {
			stop()
		}
	})
	stop()
	// The normalized cancellation outcome means the peer acknowledged it. A
	// joined cleanup/transport error still cancels the caller, but cannot certify
	// provider cancellation support for activation.
	result.Cancellation = err == GenerationCancelled
	if !result.Cancellation {
		if err != nil {
			return result, err
		}
		return result, ModelIncompatible
	}
	start(ProbeClipping)
	clipRequest := request("Produce a long output for clipping. Reply exactly with ABCDEFGHIJKLMNOPQRSTUVWXYZ.")
	clipRequest.OutputLimitBytes = 8
	long, err := p.Provider.Generate(ctx, clipRequest, nil)
	if err != nil {
		return result, err
	}
	result.Clipping = long.Clipped
	if !result.Clipping {
		return result, ModelIncompatible
	}
	if profile.Capabilities.ImageInput {
		start(ProbeImage)
		// Host-owned synthetic pixels use the same scoped resolver and provider
		// envelope as explicitly selected screenshots. No user Artifact is read.
		img := image.NewRGBA(image.Rect(0, 0, 32, 32))
		for y := 0; y < 32; y++ {
			for x := 0; x < 32; x++ {
				img.Set(x, y, color.RGBA{B: 255, A: 255})
			}
		}
		var pixels bytes.Buffer
		if png.Encode(&pixels, img) != nil {
			return result, ProviderUnavailable
		}
		r := request("Reply exactly with the dominant color of this image, as a lowercase English color name.")
		r.Snapshot.Capabilities.ImageInput = true
		r.Messages[0].ImageRefs = []string{"probe:image"}
		r.ResolveImage = func(_ context.Context, ref string) (string, []byte, error) {
			if ref != "probe:image" {
				return "", nil, ModelIncompatible
			}
			return "image/png", pixels.Bytes(), nil
		}
		seen, err := p.Provider.Generate(ctx, r, nil)
		if err != nil {
			return result, err
		}
		result.ImageInput = strings.TrimSpace(seen.Text) == "blue" && len(seen.ToolCalls) == 0 && !seen.Clipped
	}
	return result, result.ValidateCapabilities(profile.Capabilities)
}
