// Package tools declares the artifact surface. All execution uses the ordinary
// Host ToolInvocation pipeline; no model parameter carries ownership or paths.
package tools

import "github.com/z-chenhao/erii/internal/tool"

const Save = "builtin__artifact__save"
const Read = "builtin__artifact__read"
const Describe = "builtin__artifact__describe"
const List = "builtin__artifact__list"
const Publish = "builtin__artifact__publish"

func Definitions() []tool.Definition {
	defs := []tool.Definition{}
	for _, v := range []struct{ name, schema, permission string }{
		{Save, `{"type":"object","properties":{"name":{"type":"string","maxLength":255},"mediaType":{"type":"string","maxLength":128},"text":{"type":"string","maxLength":8192}},"required":["name","mediaType","text"],"additionalProperties":false}`, "artifact_write"},
		{Read, `{"type":"object","properties":{"artifactRef":{"type":"string","maxLength":256},"offset":{"type":"integer","minimum":0,"maximum":1073741824},"limit":{"type":"integer","minimum":1,"maximum":4096}},"required":["artifactRef"],"additionalProperties":false}`, "read_only"},
		{Describe, `{"type":"object","properties":{"artifactRef":{"type":"string","maxLength":256}},"required":["artifactRef"],"additionalProperties":false}`, "read_only"},
		{List, `{"type":"object","properties":{},"additionalProperties":false}`, "read_only"},
		{Publish, `{"type":"object","properties":{"artifactRefs":{"type":"array","maxItems":128,"items":{"type":"string","maxLength":256}}},"required":["artifactRefs"],"additionalProperties":false}`, "artifact_write"},
	} {
		defs = append(defs, tool.Definition{Host: true, Spec: tool.Spec{Name: v.name, InputSchema: v.schema, Version: 1, Permission: v.permission, ReplayPolicy: "safe", OutputBytes: 65536}})
	}
	return defs
}
func Known(name string) bool {
	return name == Save || name == Read || name == Describe || name == List || name == Publish
}
