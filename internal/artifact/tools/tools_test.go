package tools

import (
	"github.com/z-chenhao/erii/internal/tool"
	"github.com/z-chenhao/erii/internal/tool/codemode"
	"strings"
	"testing"
)

func TestManageSearchContract(t *testing.T) {
	defs := Definitions()
	registry, err := tool.NewRegistry(defs)
	if err != nil || len(registry.Describe()) != 2 {
		t.Fatal(registry, err)
	}
	for _, name := range []string{"builtin__artifact_save", "builtin__artifact_read", "builtin__artifact_describe", "builtin__artifact_list", "builtin__artifact_publish"} {
		if _, ok := registry.Resolve(name); ok || Known(name) {
			t.Fatal("legacy Tool exposed", name)
		}
	}
	for _, raw := range []string{
		`{"operation":"save","name":"report.txt","mediaType":"text/plain","text":"evidence"}`,
		`{"operation":"read","artifactRef":"opaque","offset":0,"limit":4096}`,
		`{"operation":"describe","artifactRef":"opaque"}`,
		`{"operation":"publish","artifactRefs":[]}`,
	} {
		if !tool.Validate(ManageSchema, raw) {
			t.Fatal("valid request rejected", raw)
		}
	}
	for _, raw := range []string{
		`{}`, `{"operation":"list"}`, `{"operation":"read"}`,
		`{"operation":"read","artifactRef":"opaque","text":"smuggled"}`,
		`{"operation":"describe","artifactRef":"opaque","artifactRefs":[]}`,
		`{"operation":"publish","artifactRefs":null}`,
		`{"operation":"publish","artifactRefs":[],"taskRef":"forged"}`,
		`{"operation":"save","name":"x","mediaType":"text/plain","text":"x","operation":"publish"}`,
	} {
		if tool.Validate(ManageSchema, raw) {
			t.Fatal("invalid request accepted", raw)
		}
	}
	for _, raw := range []string{`{}`, `{"query":"report","limit":1}`} {
		if !tool.Validate(SearchSchema, raw) {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{`{"limit":0}`, `{"limit":51}`, `{"operation":"read"}`, `{"query":null}`, `{"runId":"forged"}`} {
		if tool.Validate(SearchSchema, raw) {
			t.Fatal(raw)
		}
	}
	declaration := codemode.Describe(defs[0].Spec).Declaration
	for _, op := range []string{`"save"`, `"read"`, `"describe"`, `"publish"`} {
		if !strings.Contains(declaration, op) {
			t.Fatal(declaration)
		}
	}
	if defs[0].Spec.Permission != "artifact_write" || defs[1].Spec.Permission != "read_only" {
		t.Fatal("permissions drifted")
	}
}
