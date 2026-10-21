package llm

import (
	"context"
	"strings"
	"testing"
	"time"
)

type probeProvider struct{}

func (probeProvider) Generate(ctx context.Context, r GenerationRequest, sink func(GenerationEvent)) (GenerationResult, error) {
	// A cold local model must have time to load before the first visible token.
	if strings.Contains(r.Messages[0].Text, "Reply with OK") {
		select {
		case <-time.After(11 * time.Second):
		case <-ctx.Done():
			return GenerationResult{}, GenerationCancelled
		}
	}
	if strings.Contains(r.Messages[0].Text, "until cancelled") {
		sink(GenerationEvent{Kind: "text_delta", Text: "streaming"})
		<-ctx.Done()
		return GenerationResult{}, GenerationCancelled
	}
	if len(r.Tools) > 0 {
		return GenerationResult{ToolCalls: []ToolCall{{ID: "call", Name: "probe_echo", Arguments: `{"value":"OK"}`}}}, nil
	}
	if sink != nil {
		sink(GenerationEvent{Kind: "text_delta", Text: "OK"})
	}
	return GenerationResult{Text: "OK", Clipped: r.OutputLimitBytes > 0}, nil
}
func (probeProvider) Continue(context.Context, GenerationRequest, func(GenerationEvent)) (GenerationResult, error) {
	return GenerationResult{Text: "PROBE_RESULT_OK"}, nil
}
func (probeProvider) Cancel(context.Context, string) error  { return nil }
func (probeProvider) Dispose(context.Context, string) error { return nil }

func TestProbeColdLocalModel(t *testing.T) {
	profile := Profile{ProfileInput: ProfileInput{ProviderKind: "openai_compatible", Endpoint: "http://127.0.0.1:8000/v1", ModelID: "cold-local", ContextWindow: 262144, MaxOutputTokens: 32768}}
	result, err := (Prober{Provider: probeProvider{}}).Run(context.Background(), profile)
	if err != nil || result.Validate() != nil {
		t.Fatalf("cold model failed activation checks: %+v %v", result, err)
	}
}
