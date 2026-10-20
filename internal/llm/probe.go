package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Prober struct{ Provider LLMProvider }

// Run verifies streaming, a Tool Call/Result round trip, cancellation, and Host
// output bounds using the same LLMProvider boundary as generation.
func (p Prober) Run(ctx context.Context, profile Profile) (ProbeResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
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
	var result ProbeResult
	connection, err := p.Provider.Generate(ctx, request("Reply with OK."), func(e GenerationEvent) { result.Streaming = result.Streaming || e.Kind == "text_delta" })
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(connection.Text) == "" {
		return result, ModelIncompatible
	}
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
	r.GenerationID = id()
	r.Messages = append(r.Messages, Message{Role: "assistant", ToolCalls: called.ToolCalls}, Message{Role: "tool", Text: "PROBE_RESULT_OK", ToolCallID: called.ToolCalls[0].ID})
	continued, err := p.Provider.Continue(ctx, r, nil)
	if err != nil {
		return result, err
	}
	result.Continuation = continued.Text == "PROBE_RESULT_OK"
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
	clipRequest := request("Produce a long output for clipping. Repeat ABCDEFGHIJKLMNOPQRSTUVWXYZ 100 times.")
	clipRequest.OutputLimitBytes = 8
	long, err := p.Provider.Generate(ctx, clipRequest, nil)
	if err != nil {
		return result, err
	}
	result.Clipping = long.Clipped
	if profile.Capabilities.ImageInput {
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
