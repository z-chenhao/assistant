package tool

import (
	"context"
	"testing"
)

func TestEchoSchemaAndNoEffectfulRegistry(t *testing.T) {
	registry := EchoRegistry()
	definition, ok := registry.Resolve("builtin__text__echo")
	if !ok {
		t.Fatal("Tool absent")
	}
	for _, input := range []string{`{}`, `null`, `[]`, `{"text":1}`, `{"text":"ok","unfrozen":true}`, `{"text":"a"} {"text":"b"}`} {
		result := definition.Execute(context.Background(), input)
		if result.Error == nil || result.Error.Code != "INVALID_ARGUMENT" || result.Error.Retryable {
			t.Fatal("invalid argument accepted", input, result)
		}
	}
	result := definition.Execute(context.Background(), `{"text":"ok"}`)
	if result.Error != nil || result.Text != "ok" {
		t.Fatal(result)
	}
	for _, policy := range []string{"", "never", "query_before_retry"} {
		spec := definition.Spec
		spec.ReplayPolicy = policy
		if _, err := NewRegistry([]Definition{{Spec: spec, Execute: definition.Execute}}); err == nil {
			t.Fatal("unsafe policy admitted without M3 approval", policy)
		}
	}
	spec := definition.Spec
	spec.Permission = "external_effect"
	if _, err := NewRegistry([]Definition{{Spec: spec, Execute: definition.Execute}}); err == nil {
		t.Fatal("effectful tool admitted")
	}
}
