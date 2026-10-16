package bridge

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
)

func TestDisposeWaitsForDispatchAndFencesAdmission(t *testing.T) {
	for _, operation := range []string{"generate", "continue"} {
		t.Run(operation, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			client := openCancelDispatchPeer(t, ctx)
			p := client.p
			p.write <- struct{}{}
			held := true
			defer func() {
				if held {
					<-p.write
				}
			}()
			r := llm.GenerationRequest{GenerationID: "g", RunID: "run", SessionID: "run", Snapshot: llm.Snapshot{SnapshotID: "s", MaxOutputTokens: 10}, Messages: []llm.Message{{Role: "user", Text: strings.Repeat("x", 900000)}}}
			done, disposed := make(chan error, 1), make(chan error, 1)
			go func() {
				var err error
				if operation == "generate" {
					_, err = client.Generate(ctx, r, nil)
				} else {
					_, err = client.Continue(ctx, r, nil)
				}
				done <- err
			}()
			for {
				p.mu.Lock()
				registered := p.events[r.GenerationID] != nil
				p.mu.Unlock()
				if registered {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal("not registered")
				case <-time.After(time.Millisecond):
				}
			}
			go func() { disposed <- client.Dispose(ctx, r.SessionID) }()
			for {
				client.mu.Lock()
				fenced := client.disposedSessions[r.SessionID]
				client.mu.Unlock()
				if fenced {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal("not fenced")
				case <-time.After(time.Millisecond):
				}
			}
			// Disposal cannot acknowledge while this generation is still awaiting dispatch.
			select {
			case err := <-disposed:
				t.Fatal("disposal overtook dispatch", err)
			default:
			}
			late := r
			late.GenerationID = "late"
			if _, err := client.Generate(ctx, late, nil); err != llm.GenerationCancelled {
				t.Fatal("admission fence failed", err)
			}
			<-p.write
			held = false
			select {
			case err := <-disposed:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("disposal hung")
			}
			select {
			case err := <-done:
				if err != llm.GenerationCancelled {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("generation hung")
			}
			models, err := client.List(ctx)
			if err != nil || len(models) != 1 || models[0].ContextWindow != 0 || models[0].ModelID != "peer:1" || models[0].MaxOutputTokens != 1 {
				t.Fatal("remote dispatch/disposal order or admission drifted", models, err)
			}
		})
	}
}
