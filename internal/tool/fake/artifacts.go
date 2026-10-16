package fake

import (
	"context"
	"github.com/z-chenhao/erii/internal/tool"
	"strings"
)

func Artifacts() []tool.Definition {
	defs := []tool.Definition{}
	for _, kind := range []string{"binary", "large"} {
		name := "mcp__fixture__" + kind
		d := tool.Definition{Spec: tool.Spec{Name: name, Version: 1, InputSchema: `{"type":"object","properties":{},"additionalProperties":false}`, Permission: "read_only", ReplayPolicy: "safe", OutputBytes: 1024}}
		if kind == "binary" {
			d.Execute = func(context.Context, string) tool.Result {
				return tool.Result{Output: &tool.BinaryOutput{Name: "fixture.bin", MediaType: "application/octet-stream", Reader: strings.NewReader(strings.Repeat("BINARY-FIXTURE-", 131072))}}
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
