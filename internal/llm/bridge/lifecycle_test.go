package bridge_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/llm/bridge"
)

// Cancellation acknowledges stopping work without requiring a terminal event.
// The catalog independently exposes active work, cancellations, and process ID.
func TestLifecyclePeer(t *testing.T) {
	if len(os.Args) < 3 || os.Args[len(os.Args)-2] != "lifecycle-peer" {
		return
	}
	ledger := os.Args[len(os.Args)-1]
	scan := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	active := map[string]string{}
	sessions := map[string]string{}
	cancels := 0
	send := func(v any) {
		if enc.Encode(v) != nil {
			os.Exit(2)
		}
	}
	for scan.Scan() {
		var r struct {
			ID     uint64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(scan.Bytes(), &r) != nil {
			os.Exit(2)
		}
		reply := func(v any) { send(map[string]any{"jsonrpc": "2.0", "id": r.ID, "result": v}) }
		event := func(id string, seq int64, kind string) {
			send(map[string]any{"jsonrpc": "2.0", "method": "generation.event", "params": llm.GenerationEvent{GenerationID: id, Sequence: seq, Kind: kind, Text: "x", ErrorCode: string(llm.InvalidProviderResponse)}})
		}
		switch r.Method {
		case "bridge.initialize":
			reply(map[string]int{"protocol_version": bridge.ProtocolVersion})
		case "bridge.shutdown":
			reply(struct{}{})
			os.Exit(0)
		case "models.list":
			reply([]llm.CatalogModel{{ModelID: strconv.Itoa(os.Getpid()), ContextWindow: int64(len(active)), MaxOutputTokens: int64(cancels)}})
		case "generation.start", "generation.continue":
			var req llm.GenerationRequest
			if json.Unmarshal(r.Params, &req) != nil || len(req.Messages) != 1 {
				os.Exit(2)
			}
			f, err := os.OpenFile(ledger, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				os.Exit(2)
			}
			if _, err = f.WriteString(req.GenerationID + "\n"); err != nil || f.Close() != nil {
				os.Exit(2)
			}
			mode := req.Messages[0].Text
			active[req.GenerationID] = mode
			sessions[req.GenerationID] = req.SessionID
			if mode == "pre-ack" {
				continue
			}
			reply(struct{}{})
			switch mode {
			case "sequence", "no-ack":
				event(req.GenerationID, 2, "text_delta")
			case "kind":
				event(req.GenerationID, 1, "invalid-kind")
			case "tool":
				event(req.GenerationID, 1, "tool_call") // missing required ToolCall
			case "wire":
				if _, err := os.Stdout.WriteString("{invalid-json}\n"); err != nil {
					os.Exit(2)
				}
			case "provider-error":
				delete(active, req.GenerationID)
				event(req.GenerationID, 1, "error")
			default:
				event(req.GenerationID, 1, "text_delta")
				if mode == "completed" {
					delete(active, req.GenerationID)
					event(req.GenerationID, 2, "completed")
				}
			}
		case "session.dispose":
			var req struct {
				ID string `json:"session_id"`
			}
			if json.Unmarshal(r.Params, &req) != nil {
				os.Exit(2)
			}
			late := []string{}
			for id, mode := range active {
				if sessions[id] != req.ID {
					continue
				}
				if mode == "dispose-no-ack" {
					continue
				}
				delete(active, id)
				if mode == "late" {
					late = append(late, id)
				}
			}
			blocked := false
			for id, mode := range active {
				if sessions[id] == req.ID && mode == "dispose-no-ack" {
					blocked = true
				}
			}
			if blocked {
				continue
			}
			reply(struct{}{})
			for _, id := range late {
				event(id, 2, "text_delta")
				event(id, 3, "completed")
			}
		case "generation.cancel":
			var req struct {
				ID string `json:"generation_id"`
			}
			if json.Unmarshal(r.Params, &req) != nil {
				os.Exit(2)
			}
			cancels++
			mode := active[req.ID]
			if mode == "no-ack" {
				continue // work remains active until the Host retires this process
			}
			delete(active, req.ID)
			reply(struct{}{})
			if mode == "late" {
				event(req.ID, 2, "text_delta")
				event(req.ID, 3, "completed")
			}
		default:
			reply(struct{}{})
		}
	}
	os.Exit(0)
}

func lifecyclePeer(t *testing.T, ctx context.Context) (*bridge.Client, string) {
	t.Helper()
	ledger := filepath.Join(t.TempDir(), "generations")
	c, err := bridge.Open(ctx, os.Args[0], "-test.run=^TestLifecyclePeer$", "--", "lifecycle-peer", ledger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, ledger
}

func lifecycleState(t *testing.T, ctx context.Context, c *bridge.Client) llm.CatalogModel {
	t.Helper()
	models, err := c.List(ctx)
	if err != nil || len(models) != 1 {
		t.Fatal(models, err)
	}
	return models[0]
}

func TestHostRejectionCleansAcceptedGeneration(t *testing.T) {
	for _, mode := range []string{"sequence", "kind", "tool", "no-ack", "wire", "provider-error"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			c, ledger := lifecyclePeer(t, ctx)
			before := lifecycleState(t, ctx, c)
			r := request()
			r.Messages[0].Text = mode
			result, err := c.Generate(ctx, r, nil)
			if !errors.Is(err, llm.InvalidProviderResponse) || result.Text != "" || ctx.Err() != nil {
				t.Fatal("lost original rejection", result, err, ctx.Err())
			}
			if mode == "no-ack" && !errors.Is(err, llm.ProviderUnavailable) {
				t.Fatal("cleanup failure not preserved", err)
			}
			after := lifecycleState(t, ctx, c)
			if after.ContextWindow != 0 {
				t.Fatal("rejected generation remains active", after)
			}
			switch mode {
			case "no-ack", "wire":
				if before.ModelID == after.ModelID {
					t.Fatal("unusable peer not retired", before, after)
				}
			case "provider-error":
				if before.ModelID != after.ModelID || after.MaxOutputTokens != 0 {
					t.Fatal("provider terminal error incorrectly triggered cleanup", after)
				}
			default:
				if before.ModelID != after.ModelID || after.MaxOutputTokens != 1 {
					t.Fatal("Host rejection did not cancel reusable peer", after)
				}
			}
			data, err := os.ReadFile(ledger)
			if err != nil || string(data) != r.GenerationID+"\n" {
				t.Fatal("accepted generation replayed", string(data), err)
			}
			t.Logf("mode=%s old PID=%s new PID=%s active=%d cancels=%d", mode, before.ModelID, after.ModelID, after.ContextWindow, after.MaxOutputTokens)
		})
	}
}

func TestCancelAcknowledgementSettlesLocalGeneration(t *testing.T) {
	for _, mode := range []string{"ack-only", "pre-ack", "late", "completed"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			c, _ := lifecyclePeer(t, ctx)
			r := request()
			r.Messages[0].Text = mode
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			releaseSink := func() { once.Do(func() { close(release) }) }
			defer releaseSink()
			type outcome struct {
				result llm.GenerationResult
				err    error
				count  int
			}
			done := make(chan outcome, 1)
			go func() {
				count := 0
				result, err := c.Generate(ctx, r, func(e llm.GenerationEvent) {
					count++
					if mode == "completed" && e.Kind == "completed" {
						close(entered)
						<-release
					} else if count == 1 && mode != "completed" {
						close(entered)
						if mode == "late" {
							<-release
						}
					}
				})
				done <- outcome{result, err, count}
			}()
			if mode == "pre-ack" {
				// Catalog RPC is a barrier after start dispatch; the peer deliberately
				// never acknowledges start. The caller context stays live throughout.
				for lifecycleState(t, ctx, c).ContextWindow == 0 {
					if ctx.Err() != nil {
						t.Fatal(ctx.Err())
					}
				}
			} else {
				select {
				case <-entered:
				case <-ctx.Done():
					t.Fatal("generation did not stream")
				}
			}
			if err := c.Cancel(ctx, r.GenerationID); err != nil {
				t.Fatal(err)
			}
			state := lifecycleState(t, ctx, c) // also a barrier after late events
			if state.ContextWindow != 0 || state.MaxOutputTokens != 1 {
				t.Fatal(state)
			}
			releaseSink()
			select {
			case got := <-done:
				wantCount := 1
				if mode == "pre-ack" {
					wantCount = 0
				}
				if mode == "completed" {
					wantCount = 2
				}
				if !errors.Is(got.err, llm.GenerationCancelled) || got.result.Text != "" || got.count != wantCount || ctx.Err() != nil {
					t.Fatal("acknowledged cancellation did not fence local result", got, ctx.Err())
				}
			case <-time.After(250 * time.Millisecond):
				t.Fatal("Generate still waits after cancel acknowledgement")
			}
		})
	}
}

func TestCancelOnlySettlesMatchingLocalGeneration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _ := lifecyclePeer(t, ctx)
	start := func(id string) <-chan error {
		entered, done := make(chan struct{}), make(chan error, 1)
		r := request()
		r.GenerationID = id
		go func() {
			_, err := c.Generate(ctx, r, func(llm.GenerationEvent) { close(entered) })
			done <- err
		}()
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("generation did not stream")
		}
		return done
	}
	one, two := start("one"), start("two")
	if err := c.Cancel(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-one:
		if !errors.Is(err, llm.GenerationCancelled) {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("matching call did not settle")
	}
	if err := c.Cancel(ctx, "unregistered"); err != nil {
		t.Fatal(err)
	}
	state := lifecycleState(t, ctx, c)
	if state.ContextWindow != 1 {
		t.Fatal("unrelated generation stopped", state)
	}
	select {
	case err := <-two:
		t.Fatal("unrelated local generation settled", err)
	default:
	}
	if err := c.Cancel(ctx, "two"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-two:
		if !errors.Is(err, llm.GenerationCancelled) {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("second matching call did not settle")
	}
}
