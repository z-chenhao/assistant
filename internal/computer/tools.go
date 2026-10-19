package computer

import "github.com/z-chenhao/erii/internal/tool"

// Definitions keeps the established private cua bridge. Lifecycle policy is
// Host-owned; no ensure/stop/restore operation is exposed as a model parameter.
// Surface binding/actions are supplied by the subsequent Computer Use slice.
func Definitions(resourceHash string) []tool.Definition {
	var out []tool.Definition
	for _, operation := range []string{"discover", "bind", "invoke"} {
		out = append(out, tool.Definition{Host: true, Spec: tool.Spec{Internal: true, Name: "builtin__computer__" + operation, Version: 1, Description: "Computer bridge; use cua facade. This lifecycle build supports cua.getState().", InputSchema: `{"type":"object","properties":{"method":{"type":"string","maxLength":64},"args":{},"handle_token":{"type":"string","maxLength":128}},"required":["method"],"additionalProperties":false}`, ResourceHash: resourceHash, Permission: "read_only", ReplayPolicy: "safe", TimeoutMS: 300000}})
	}
	return out
}
