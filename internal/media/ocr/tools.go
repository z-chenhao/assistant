package ocr

import "github.com/z-chenhao/erii/internal/tool"

const Name = "builtin__image_ocr"
const Schema = `{"type":"object","properties":{"artifactRef":{"type":"string","minLength":9,"maxLength":256}},"required":["artifactRef"],"additionalProperties":false}`

func Definition(hash string) tool.Definition {
	return tool.Definition{Host: true, Spec: tool.Spec{Name: Name, Version: 1, InputSchema: Schema,
		Description:  "Extract local OCR text and word boxes from a scoped PNG/JPEG. Transcription evidence, not scene understanding. Full output is an Artifact.",
		ResourceHash: hash, Permission: "read_only", ReplayPolicy: "safe", TimeoutMS: 30000, OutputBytes: 65536}}
}
