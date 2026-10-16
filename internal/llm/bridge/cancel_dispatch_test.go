package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
)

func TestCancelDispatchPeer(t *testing.T) {
	if os.Args[len(os.Args)-1] != "cancel-dispatch-peer" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), MaxFrameBytes)
	enc := json.NewEncoder(os.Stdout)
	var cancelID uint64
	active, starts, cancels := 0, 0, 0
	for scanner.Scan() {
		var r struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &r) != nil {
			os.Exit(2)
		}
		reply := func(id uint64, v any) {
			if enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": v}) != nil {
				os.Exit(2)
			}
		}
		switch r.Method {
		case "bridge.initialize":
			reply(r.ID, map[string]int{"protocol_version": 1})
		case "generation.cancel":
			cancels++
			if active == 0 {
				cancelID = r.ID
			} else {
				active = 0
				reply(r.ID, struct{}{})
			}
		case "generation.start", "generation.continue":
			active++
			starts++
			reply(r.ID, struct{}{})
			if cancelID != 0 {
				reply(cancelID, struct{}{})
				cancelID = 0
			}
		case "models.list":
			reply(r.ID, []llm.CatalogModel{{ModelID: "peer:" + strconv.Itoa(starts), ContextWindow: int64(active), MaxOutputTokens: int64(cancels)}})
		case "bridge.shutdown":
			reply(r.ID, struct{}{})
			os.Exit(0)
		}
	}
	os.Exit(0)
}

func openCancelDispatchPeer(t *testing.T, ctx context.Context) *Client {
	t.Helper()
	c, err := Open(ctx, os.Args[0], "-test.run=^TestCancelDispatchPeer$", "--", "cancel-dispatch-peer")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestCancelOrdersAfterGenerationDispatch(t *testing.T) {
	for _, method := range []string{"generate", "continue"} {
		t.Run(method, func(t *testing.T) {
			for attempt := 0; attempt < 3; attempt++ {
				func() {
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()
					c := openCancelDispatchPeer(t, ctx)
					defer c.Close()
					done := make(chan error, 1)
					r := llm.GenerationRequest{GenerationID: "g", RunID: "r", SessionID: "r", Snapshot: llm.Snapshot{SnapshotID: "s", MaxOutputTokens: 10}, Messages: []llm.Message{{Role: "user", Text: strings.Repeat("x", 900000)}}}
					go func() {
						var err error
						if method == "generate" {
							_, err = c.Generate(ctx, r, nil)
						} else {
							_, err = c.Continue(ctx, r, nil)
						}
						done <- err
					}()
					// Registration is observable before request encoding and writer admission.
					// The valid large frame makes cancellation compete with those phases.
					for {
						c.p.mu.Lock()
						registered := c.p.events["g"] != nil
						c.p.mu.Unlock()
						if registered {
							break
						}
						if ctx.Err() != nil {
							t.Fatal(ctx.Err())
						}
					}
					if err := c.Cancel(ctx, "g"); err != nil {
						t.Fatal(err)
					}
					select {
					case err := <-done:
						if !errors.Is(err, llm.GenerationCancelled) {
							t.Fatal(err)
						}
					case <-ctx.Done():
						t.Fatal("acknowledged cancellation did not settle generation")
					}
					models, err := c.List(ctx)
					if err != nil || len(models) != 1 {
						t.Fatal(models, err)
					}
					if models[0].ContextWindow != 0 || models[0].MaxOutputTokens != 1 || models[0].ModelID != "peer:1" || ctx.Err() != nil {
						t.Fatalf("attempt %d: remote work leaked, replayed or required redundant cleanup: %+v caller=%v", attempt, models, ctx.Err())
					}
				}()
			}
		})
	}
}

func TestCancelDispatchWaitHonorsDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c := openCancelDispatchPeer(t, ctx)
	// Hold a registered request before dispatch. No wire cancellation may overtake
	// it, and the waiting Cancel must respect its own caller's deadline.
	stream := newEventStream("held", 10)
	c.p.mu.Lock()
	c.p.events["held"] = stream
	c.p.mu.Unlock()
	wait, stop := context.WithTimeout(ctx, 20*time.Millisecond)
	defer stop()
	if err := c.Cancel(wait, "held"); !errors.Is(err, llm.ProviderUnavailable) || wait.Err() == nil {
		t.Fatal(err, wait.Err())
	}
	models, err := c.List(ctx)
	if err != nil || len(models) != 1 || models[0].MaxOutputTokens != 0 || stream.wasCancelled() {
		t.Fatal("early wire cancellation or local settlement", models, err)
	}
	// Close must remain independent of the registration's unreleased dispatch.
	if err := c.Close(); err != nil || ctx.Err() != nil {
		t.Fatal(err, ctx.Err())
	}
}
