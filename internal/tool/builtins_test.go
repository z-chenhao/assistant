package tool

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/z-chenhao/collie/internal/llm"
)

func TestLegacyBuiltinSnapshotIsRejectedWithoutAliasesOrMutation(t *testing.T) {
	current := Spec{Name: "builtin__task_search", Version: 1, InputSchema: `{"type":"object"}`, Permission: "read_only", ReplayPolicy: "safe"}
	r, err := NewRegistry([]Definition{{Host: true, Spec: current}})
	if err != nil {
		t.Fatal(err)
	}
	old := current
	old.Name = "builtin__task__search"
	frozen := []Spec{old}
	before := append([]Spec(nil), frozen...)
	if _, ok := r.Resolve(old.Name); ok {
		t.Fatal("old tool name became a live alias")
	}
	if _, err := r.Select(frozen); !errors.Is(err, llm.ModelIncompatible) {
		t.Fatal("old frozen snapshot silently renamed", err)
	}
	if !reflect.DeepEqual(frozen, before) || r.Describe()[0].Name != current.Name {
		t.Fatal("selection rewrote frozen or current metadata")
	}
}

func TestHostCatalogRejectsUnknownNamesPermissionsAndProviderHandlers(t *testing.T) {
	for name, policy := range builtinCatalog {
		d := Definition{Host: true, Spec: Spec{Name: name, Version: 1, InputSchema: `{"type":"object"}`, Permission: policy.permission}}
		if !validDefinition(d) {
			t.Fatal("catalog entry cannot register", name)
		}
		if family, ok := HostFamily(name); !ok || family != policy.family {
			t.Fatal("catalog dispatch drift", name)
		}
		wrong := d
		wrong.Spec.Permission = "unbounded_permission"
		if validDefinition(wrong) {
			t.Fatal("Host permission escalated", name)
		}
		wrong = d
		wrong.Execute = func(context.Context, string) Result { return Result{} }
		if validDefinition(wrong) {
			t.Fatal("Host external handler accepted", name)
		}
		wrong = d
		wrong.Invoke = func(context.Context, Request) Outcome { return Outcome{} }
		if validDefinition(wrong) {
			t.Fatal("Host effect handler accepted", name)
		}
		if !policy.internal {
			wrong = d
			wrong.Spec.Internal = true
			if validDefinition(wrong) {
				t.Fatal("invalid internal projection", name)
			}
		}
	}
	if _, ok := HostFamily("builtin__unknown_execute"); ok {
		t.Fatal("unknown builtin dispatched")
	}
	if validDefinition(Definition{Host: true, Spec: Spec{Name: "builtin__unknown_execute", Permission: "read_only"}}) {
		t.Fatal("unknown Host registered")
	}
}
