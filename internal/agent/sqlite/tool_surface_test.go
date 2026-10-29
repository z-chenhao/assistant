package sqlite

import (
	"context"
	"github.com/z-chenhao/collie/internal/llm"
	"github.com/z-chenhao/collie/internal/memory"
	"github.com/z-chenhao/collie/internal/recall"
	"github.com/z-chenhao/collie/internal/tool"
	"testing"
)

func TestUnconfiguredMemoryFailsWithoutMutationOrAutomaticRecall(t *testing.T) {
	ctx := context.Background()
	defs := []tool.Definition{memory.Definition(), recall.Definition(recall.UnconfiguredIdentity)}
	calls := []llm.ToolCall{
		{ID: "remember", Name: memory.ManageName, Arguments: `{"operation":"remember","content":"I prefer tea"}`},
		{ID: "forget", Name: memory.ManageName, Arguments: `{"operation":"forget","memory_ref":"memory:missing"}`},
		{ID: "search", Name: recall.ToolName, Arguments: `{"query":"tea"}`},
	}
	s, r, _, e, exchange := admitPolicy(t, defs, calls)
	out, err := e.Execute(ctx, exchange, calls)
	if err != nil || len(out) != 3 {
		t.Fatal(out, err)
	}
	for _, result := range out {
		if result.Error == nil || result.Error.Code != "PROVIDER_UNAVAILABLE" {
			t.Fatal(result)
		}
	}
	for _, table := range []string{"memory_mutation", "memory_privacy", "run_recall", "approval"} {
		if scalar(t, s, "SELECT count(*) FROM "+table) != 0 {
			t.Fatal("unconfigured call changed state", table)
		}
	}
	if r.recall != nil {
		t.Fatal("implicit recall provider")
	}
}
