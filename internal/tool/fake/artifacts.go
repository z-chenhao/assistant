package fake

import (
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/z-chenhao/collie/internal/tool"
)

func Artifacts() []tool.Definition {
	defs := []tool.Definition{}
	for _, kind := range []string{"binary", "large", "slow_binary"} {
		name := "mcp__fixture__" + kind
		d := tool.Definition{Spec: tool.Spec{Name: name, Version: 1, InputSchema: `{"type":"object","properties":{},"additionalProperties":false}`, Permission: "read_only", ReplayPolicy: "safe", OutputBytes: 1024}}
		if kind == "binary" {
			d.Execute = func(context.Context, string) tool.Result {
				return tool.Result{Output: &tool.BinaryOutput{Name: "fixture.bin", MediaType: "application/octet-stream", Reader: strings.NewReader(strings.Repeat("BINARY-FIXTURE-", 131072))}}
			}
		} else if kind == "slow_binary" {
			d.Spec.TimeoutMS = 20
			d.Execute = func(context.Context, string) tool.Result {
				return tool.Result{Output: &tool.BinaryOutput{Name: "slow.bin", MediaType: "application/octet-stream", Reader: &delayedArtifact{closed: make(chan struct{})}}}
			}
		} else {
			d.Execute = func(context.Context, string) tool.Result {
				return tool.Result{Text: strings.Repeat("LARGE-FIXTURE-", 16384)}
			}
		}
		defs = append(defs, d)
	}
	return defs
}

// A complete stream is deliberately later than its Tool deadline. Close must
// unblock Read; the maintained CLI harness proves the Host owns that lifecycle.
type delayedArtifact struct {
	closed chan struct{}
	once   sync.Once
	read   bool
}

func (r *delayedArtifact) Read(p []byte) (int, error) {
	if r.read {
		return 0, io.EOF
	}
	r.read = true
	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-r.closed:
		return 0, io.ErrClosedPipe
	case <-timer.C:
		p[0] = 'x'
		return 1, nil
	}
}
func (r *delayedArtifact) Close() error { r.once.Do(func() { close(r.closed) }); return nil }
