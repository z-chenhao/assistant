package llm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

type probeProvider struct {
	before   func(context.Context, GenerationRequest) error
	disposed bool
}

func (p *probeProvider) Generate(ctx context.Context, r GenerationRequest, sink func(GenerationEvent)) (GenerationResult, error) {
	if p.before != nil {
		if err := p.before(ctx, r); err != nil {
			return GenerationResult{}, err
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
func (p *probeProvider) Continue(ctx context.Context, r GenerationRequest, sink func(GenerationEvent)) (GenerationResult, error) {
	if p.before != nil {
		if err := p.before(ctx, r); err != nil {
			return GenerationResult{}, err
		}
	}
	return GenerationResult{Text: "PROBE_RESULT_OK"}, nil
}
func (*probeProvider) Cancel(context.Context, string) error    { return nil }
func (p *probeProvider) Dispose(context.Context, string) error { p.disposed = true; return nil }

func probeProfile() Profile {
	return Profile{ProfileInput: ProfileInput{ProviderKind: "openai_compatible", Endpoint: "http://127.0.0.1:8000/v1", ModelID: "cold-local", ContextWindow: 262144, MaxOutputTokens: 32768}}
}

func TestProbeColdLocalModel(t *testing.T) {
	profile := probeProfile()
	provider := &probeProvider{before: func(ctx context.Context, r GenerationRequest) error {
		if r.Snapshot.MaxOutputTokens != profile.MaxOutputTokens || r.Snapshot.ContextWindow != profile.ContextWindow {
			t.Fatal("probe changed configured limits")
		}
		if strings.Contains(r.Messages[0].Text, "Reply with OK") {
			select {
			case <-time.After(11 * time.Second):
			case <-ctx.Done():
				return GenerationCancelled
			}
		}
		return nil
	}}
	var stages []ProbeStage
	result, err := (Prober{Provider: provider}).RunWithProgress(context.Background(), profile, func(s ProbeStage) { stages = append(stages, s) })
	if err != nil || result.Validate() != nil {
		t.Fatalf("cold model failed activation checks: %+v %v", result, err)
	}
	if !provider.disposed || !reflect.DeepEqual(stages, []ProbeStage{ProbeConnection, ProbeTools, ProbeContinuation, ProbeCancellation, ProbeClipping}) {
		t.Fatalf("missing stage or cleanup: %v disposed=%v", stages, provider.disposed)
	}
}

func TestProbeDeadlineAndCallerCancellation(t *testing.T) {
	for _, stage := range []ProbeStage{ProbeConnection, ProbeTools, ProbeContinuation, ProbeCancellation, ProbeClipping} {
		for _, callerCancels := range []bool{false, true} {
			t.Run(stage.String()+"/caller="+fmt.Sprint(callerCancels), func(t *testing.T) {
				parent, cancel := context.WithCancel(context.Background())
				defer cancel()
				current := ProbeConnection
				provider := &probeProvider{before: func(ctx context.Context, _ GenerationRequest) error {
					if current == stage {
						if callerCancels {
							cancel()
						}
						<-ctx.Done()
						return GenerationCancelled
					}
					return nil
				}}
				result, err := (Prober{Provider: provider, stepTimeout: 20 * time.Millisecond}).RunWithProgress(parent, probeProfile(), func(s ProbeStage) { current = s })
				var detail *ProbeError
				if !errors.As(err, &detail) || detail.Stage != stage || (detail.Timeout > 0) == callerCancels || !provider.disposed {
					t.Fatalf("wrong failure/cleanup: %+v %v disposed=%v", detail, err, provider.disposed)
				}
				if !callerCancels && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal(err)
				}
				if stage == ProbeCancellation && result.Cancellation {
					t.Fatal("deadline certified cancellation")
				}
			})
		}
	}
}

func TestProbeErrorSanitizesCause(t *testing.T) {
	err := &ProbeError{Stage: ProbeTools, Cause: errors.Join(AuthRequired, errors.New("PRIVATE_PROVIDER_BODY"))}
	if !errors.Is(err, AuthRequired) || strings.Contains(err.Error(), "PRIVATE") || !strings.Contains(err.Error(), "AUTH_REQUIRED") {
		t.Fatal(err)
	}
}

func TestProbeChecksHaveIndependentDeadlines(t *testing.T) {
	provider := &probeProvider{before: func(ctx context.Context, _ GenerationRequest) error {
		select {
		case <-time.After(100 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return GenerationCancelled
		}
	}}
	result, err := (Prober{Provider: provider, stepTimeout: 300 * time.Millisecond}).Run(context.Background(), probeProfile())
	if err != nil || result.Validate() != nil {
		t.Fatalf("shared deadline or failed check: %+v %v", result, err)
	}
}
