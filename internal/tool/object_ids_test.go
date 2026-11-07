package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestObjectIDsPreserveEvidenceAndReplayRepresentation(t *testing.T) {
	const original = `{"ref":"conversation:message","artifact_refs":["artifact:file"],"content":{"history_artifact":"artifact:archive","text":"artifact:quoted"},"url":"https://example.test/file","number":9007199254740993}`
	projected := ModelObjectIDs(original)
	for _, s := range []string{`"ref":"message"`, `"artifact_refs":["file"]`, `"history_artifact":"archive"`, `"text":"artifact:quoted"`, `https://example.test/file`, `9007199254740993`} {
		if !strings.Contains(projected, s) {
			t.Fatal("identity projection changed evidence or lost an ID", projected)
		}
	}
	r := Result{ObjectIDs: "host", Text: original, ArtifactRefs: []string{"artifact:file"}}
	model := r.ModelText()
	if strings.Contains(model, "object_ids") || strings.Contains(model, "artifact:file") || r.ArtifactRefs[0] != "artifact:file" || r.Text != original {
		t.Fatal("projection changed storage or leaked Host keys", model, r)
	}
	raw, _ := json.Marshal(r)
	var replay Result
	if json.Unmarshal(raw, &replay) != nil || replay.ModelText() != model {
		t.Fatal("presentation changed on replay", replay)
	}
	foreign := Result{ObjectIDs: "artifacts", Text: `{"ref":"task:remote"}`, ArtifactRefs: []string{"artifact:file"}}
	if !strings.Contains(foreign.ModelText(), "task:remote") || strings.Contains(foreign.ModelText(), "artifact:file") {
		t.Fatal("rewrote an external protocol or leaked a Host artifact", foreign.ModelText())
	}
	legacy := Result{Text: original, ArtifactRefs: []string{"artifact:file"}}
	if !strings.Contains(legacy.ModelText(), "artifact:file") {
		t.Fatal("historical receipt changed")
	}
	for _, text := range []string{"literal artifact:file", `{"ref":"artifact:file"} trailing`, `{"text":"conversation:quoted","path":"/tmp/task:file"}`} {
		if ModelObjectIDs(text) != text {
			t.Fatal("rewrote original text", text)
		}
	}
}

func TestObjectIDsKeepOpaqueStateArgumentsAndAmbiguousJSON(t *testing.T) {
	const raw = `{"source_ref":"artifact:archive","components":[{"component_ref":"ui:widget#0","context":{"ref":"artifact:literal","nested":{"task_ref":"task:user-data"}}}],"arguments":{"artifactRef":"artifact:exact-action"},"input_schema":{"default":{"ref":"task:external"}}}`
	projected := ModelObjectIDs(raw)
	for _, want := range []string{`"source_ref":"archive"`, `"component_ref":"widget#0"`, `"ref":"artifact:literal"`, `"task_ref":"task:user-data"`, `"artifactRef":"artifact:exact-action"`, `"ref":"task:external"`} {
		if !strings.Contains(projected, want) {
			t.Fatal("rewrote opaque evidence or left a Host ID", projected)
		}
	}
	const duplicate = `{"ref":"artifact:file","evidence":{"value":1,"value":2}}`
	if got := ModelObjectIDs(duplicate); got != duplicate {
		t.Fatal("projection collapsed ambiguous evidence", got)
	}
}
