package bridge

import (
	"context"
	"errors"
	"github.com/z-chenhao/collie/internal/llm"
	"testing"
)

func TestPrivacyGuardRejectsBeforeProviderPipeDispatch(t *testing.T) {
	ctx := context.Background()
	c := openCancelDispatchPeer(t, ctx)
	want := errors.New("revoked")
	r := llm.GenerationRequest{GenerationID: "guarded", RunID: "run", SessionID: "run", Snapshot: llm.Snapshot{SnapshotID: "snapshot", MaxOutputTokens: 1024}, Messages: []llm.Message{{Role: "user", Text: "private context"}}}
	r.DispatchGuard = func(context.Context) (func(), error) { return nil, want }
	if _, e := c.Generate(ctx, r, nil); !errors.Is(e, want) {
		t.Fatal(e)
	}
	models, e := c.List(ctx)
	if e != nil || len(models) != 1 || models[0].ModelID != "peer:0" {
		t.Fatal("revoked bytes dispatched", models, e)
	}
}
