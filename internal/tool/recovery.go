package tool

import (
	"context"
	"io"
	"time"
)

// closeOutput retains ownership until consumption ends, including rejected
// evidence. Cancellation closes a blocked provider stream and cleanup joins it.
func closeOutput(ctx context.Context, result Result) func() {
	if result.Output != nil {
		if closer, ok := result.Output.Reader.(io.Closer); ok {
			closed := make(chan struct{})
			stop := context.AfterFunc(ctx, func() { _ = closer.Close(); close(closed) })
			return func() {
				if !stop() {
					<-closed
				} else {
					_ = closer.Close()
				}
			}
		}
	}
	return func() {}
}

// RecoverResult queries authoritative evidence and consumes its receipt under
// one frozen deadline. settle must atomically normalize and commit the result;
// it returns UNKNOWN_SIDE_EFFECT when the receipt cannot be safely retained.
// This function never dispatches an effect.
func RecoverResult(ctx context.Context, d Definition, request Request, settle func(context.Context, Result) (Result, error)) (Evidence, error) {
	p := d.Spec.Policy()
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(p.TimeoutMS)*time.Millisecond)
	defer cancel()
	evidence := d.Query(queryCtx, request)
	defer closeOutput(queryCtx, evidence.Result)()
	if queryCtx.Err() != nil {
		return Evidence{State: "unknown"}, nil
	}
	if evidence.State != "succeeded" && evidence.State != "failed" {
		return evidence, nil
	}
	result := Normalize(evidence.Result)
	if (evidence.State == "succeeded") != (result.Error == nil) || !result.terminal() || len(result.Text) > p.OutputBytes {
		return Evidence{State: "unknown"}, nil
	}
	result, err := settle(queryCtx, result)
	if err != nil {
		return Evidence{}, err
	}
	if !result.terminal() {
		return Evidence{State: "unknown"}, nil
	}
	return Evidence{State: evidence.State, Result: result}, nil
}
