package codemode

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/z-chenhao/erii/internal/tool"
)

var ErrTransport = errors.New("javascript runtime transport unavailable")

// ExecutionHost is CodeMode's consumer-owned durable boundary. The owning
// adapter enforces claims, frozen source, atomic acknowledgments and replay.
// The runtime coordinator cannot access SQLite, providers or model credentials.
type ExecutionHost interface {
	ExecutionEventSink
	Wake(context.Context) (int64, error)
	LoadOutcome(context.Context) (Outcome, bool, error)
	BeginAttempt(context.Context) error
	SaveOutcome(context.Context, Outcome) error
	Drain(context.Context) error
	Release(context.Context, int64) (bool, error)
	Idle(context.Context) (tool.Result, error)
}

// Run rebuilds the VM only after transport death, always from the same frozen
// request. Nested calls continue through the existing Tool Executor and Host.
func Run(ctx context.Context, runtime JavaScriptRuntime, request ExecutionRequest, host ExecutionHost) (tool.Result, error) {
	crashes := 0
	for {
		observed, err := host.Wake(ctx)
		if err != nil {
			return tool.Result{}, err
		}
		outcome, saved, err := host.LoadOutcome(ctx)
		if err != nil {
			return tool.Result{}, err
		}
		if !saved {
			if err = host.BeginAttempt(ctx); err != nil {
				return tool.Result{}, err
			}
			outcome, err = startAttempt(ctx, runtime, request, host)
			if err != nil {
				if errors.Is(err, ErrEventCommit) {
					return tool.Failure("PROVIDER_UNAVAILABLE"), nil
				}
				if ctx.Err() != nil {
					return tool.Result{}, ctx.Err()
				}
				select {
				case <-tool.ShutdownSignal(ctx):
					return tool.Result{}, context.Canceled
				default:
				}
				crashes++
				if crashes <= 3 {
					continue
				}
				return tool.Result{}, errors.Join(ErrTransport, err)
			}
		}
		if outcome.Kind == "completed" || outcome.Kind == "failed" {
			if err = host.SaveOutcome(ctx, outcome); err != nil {
				return tool.Result{}, err
			}
		}
		err = host.Drain(ctx)
		if err != nil && !errors.Is(err, tool.ErrSuspended) {
			return tool.Result{}, err
		}
		if errors.Is(err, tool.ErrSuspended) {
			idle, err := host.Release(ctx, observed)
			if err != nil {
				return tool.Result{}, err
			}
			if !idle {
				continue
			}
			return tool.Result{}, tool.ErrSuspended
		}
		if outcome.Kind != "suspended" {
			result, err := host.Idle(ctx)
			if err != nil || result.Error != nil {
				return result, err
			}
			return outcomeResult(outcome), nil
		}
	}
}

func startAttempt(ctx context.Context, runtime JavaScriptRuntime, request ExecutionRequest, host ExecutionEventSink) (Outcome, error) {
	vmCtx, stopVM := context.WithCancel(ctx)
	defer stopVM()
	joined, watcherDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-tool.ShutdownSignal(ctx):
			stopVM()
		case <-joined:
		}
	}()
	outcome, err := runtime.Start(vmCtx, request, host)
	close(joined)
	<-watcherDone
	select {
	case <-tool.ShutdownSignal(ctx):
		return Outcome{}, context.Canceled
	default:
	}
	return outcome, err
}

func outcomeResult(outcome Outcome) tool.Result {
	switch outcome.Kind {
	case "cancelled":
		return tool.Failure("CANCELLED")
	case "failed":
		if outcome.Error != nil {
			return tool.Normalize(tool.Result{Error: outcome.Error})
		}
		return tool.Failure("INVALID_ARGUMENT")
	case "completed":
		value, _ := json.Marshal(outcome.Output)
		result := tool.Result{Text: string(value)}
		for _, raw := range outcome.Output {
			var item struct {
				Type     string
				Data     string
				MimeType string
			}
			if json.Unmarshal(raw, &item) == nil && item.Type == "image" && item.MimeType == "application/x-erii-artifact" {
				result.ImageRefs = append(result.ImageRefs, item.Data)
				result.ArtifactRefs = append(result.ArtifactRefs, item.Data)
			}
		}
		return result
	}
	return tool.Failure("PROVIDER_UNAVAILABLE")
}
