package tool

import (
	"context"
	"sync"
	"testing"
)

func TestRegistryGroupReplacementKeepsSnapshotsAndOtherGroups(t *testing.T) {
	r := FixtureRegistry()
	d, _ := r.Resolve("mcp__fixture__echo")
	d.Spec.Name = "mcp__apps_one__search_tools"
	if err := r.ReplaceGroup("mcp__apps_", []Definition{d}); err != nil {
		t.Fatal(err)
	}
	frozen := r.Describe()
	selected, err := r.Select(frozen)
	if err != nil {
		t.Fatal(err)
	}
	d.Spec.Name = "mcp__apps_two__search_tools"
	if err = r.ReplaceGroup("mcp__apps_", []Definition{d}); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Resolve("mcp__fixture__echo"); !ok {
		t.Fatal("unrelated capability removed")
	}
	if _, ok := selected.Resolve(d.Spec.Name); ok {
		t.Fatal("frozen capability expanded")
	}
	if _, err = r.Select(frozen); err == nil {
		t.Fatal("revoked spec accepted")
	}
	if err = r.ReplaceGroup("mcp__apps_", []Definition{{Spec: Spec{Name: "bad"}}}); err == nil {
		t.Fatal("invalid replacement accepted")
	}
	if _, ok := r.Resolve(d.Spec.Name); !ok {
		t.Fatal("failed replacement was not atomic")
	}
	other := d
	other.Spec.Name = "mcp__other__tool"
	if err = r.ReplaceGroup("mcp__apps_", []Definition{other}); err == nil {
		t.Fatal("escaped group")
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 100 {
				r.Describe()
				r.Resolve(d.Spec.Name)
			}
		})
	}
	for range 100 {
		if err = r.ReplaceGroup("mcp__apps_", []Definition{d}); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
	old, _ := selected.Resolve("mcp__apps_one__search_tools")
	if old.Execute(context.Background(), `{"text":"frozen"}`).Text != "frozen" {
		t.Fatal("frozen executor changed")
	}
}
