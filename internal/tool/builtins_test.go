package tool

import (
	"context"
	"testing"
)

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
	if _, ok := HostFamily("builtin__unknown__execute"); ok {
		t.Fatal("unknown builtin dispatched")
	}
	if validDefinition(Definition{Host: true, Spec: Spec{Name: "builtin__unknown__execute", Permission: "read_only"}}) {
		t.Fatal("unknown Host registered")
	}
}
