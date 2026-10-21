package computer

import "github.com/z-chenhao/erii/internal/tool"

// Definitions keeps the established private cua bridge. Lifecycle policy is
// Host-owned; no ensure/stop/restore operation is exposed as a model parameter.
// Every facade operation uses the same accepted invocation pipeline.
func Definitions(resourceHash string) []tool.Definition {
	var out []tool.Definition
	for _, operation := range []string{"discover", "bind", "invoke"} {
		out = append(out, tool.Definition{Host: true, Spec: tool.Spec{Internal: true, Name: "builtin__computer__" + operation, Version: 1, Description: "Computer bridge; use cua facade and bound documentation().", InputSchema: `{"type":"object","properties":{"method":{"type":"string","maxLength":64},"args":{},"handle_token":{"type":"string","maxLength":128}},"required":["method"],"additionalProperties":false}`, ResourceHash: resourceHash + ":computer-use-v1", Permission: "read_only", ReplayPolicy: "safe", TimeoutMS: 300000, OutputBytes: 1 << 20}})
	}
	return out
}
