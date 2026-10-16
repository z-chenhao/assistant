package bridge_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/llm/bridge"
)

// A real peer emits the entire burst before reading another RPC. Its catalog
// reports the last emitted sequence and cancellation count, providing a barrier
// independent of the Host sink. Some bursts precede even the start ack.
func TestBurstPeer(t *testing.T) {
	if os.Args[len(os.Args)-1] != "burst-peer" {
		return
	}
	scan := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	send := func(v any) {
		if enc.Encode(v) != nil {
			os.Exit(2)
		}
	}
	last, cancellations := 0, 0
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
		switch r.Method {
		case "bridge.initialize":
			reply(map[string]int{"protocol_version": bridge.ProtocolVersion})
		case "bridge.shutdown":
			reply(struct{}{})
			os.Exit(0)
		case "models.list":
			reply([]llm.CatalogModel{{ModelID: strconv.Itoa(last), MaxOutputTokens: int64(cancellations)}})
		case "generation.cancel":
			cancellations++
			reply(struct{}{})
		case "generation.start":
			var req llm.GenerationRequest
			if json.Unmarshal(r.Params, &req) != nil || len(req.Messages) != 1 {
				os.Exit(2)
			}
			mode := req.Messages[0].Text
			count := 128
			if mode != "small" {
				count = 4095
			}
			if mode == "overflow" {
				count = 4096
			}
			if mode != "before-ack" {
				reply(struct{}{})
			}
			for i := 1; i <= count+1; i++ {
				e := llm.GenerationEvent{GenerationID: req.GenerationID, Sequence: int64(i), Kind: "text_delta", Text: "x"}
				if i == count+1 {
					e.Kind, e.Text = "completed", ""
				}
				wire := map[string]any{"jsonrpc": "2.0", "method": "generation.event", "params": e}
				send(wire)
				if mode == "duplicates" {
					send(wire)
				}
				last = i
			}
			if mode == "before-ack" {
				reply(struct{}{})
			}
		default:
			reply(struct{}{})
		}
	}
	os.Exit(0)
}

func openBurst(t *testing.T, ctx context.Context) *bridge.Client {
	t.Helper()
	c, err := bridge.Open(ctx, os.Args[0], "-test.run=^TestBurstPeer$", "--", "burst-peer")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestValidBurstSlowSink(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c := openBurst(t, ctx)
	r := request()
	r.Messages[0].Text = "small"
	count := 0
	result, err := c.Generate(ctx, r, func(e llm.GenerationEvent) {
		count++
		if e.Sequence != int64(count) {
			t.Errorf("sink sequence=%d want=%d", e.Sequence, count)
		}
		time.Sleep(time.Millisecond)
	})
	if err != nil || result.Text != strings.Repeat("x", 128) || count != 129 {
		t.Fatal("valid burst rejected or dropped", result, count, err)
	}
}

func TestBurstMailboxRPCAndCancellation(t *testing.T) {
	for _, mode := range []string{"maximum", "duplicates", "before-ack", "overflow", "cancel", "close"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			c := openBurst(t, ctx)
			r := request()
			r.Messages[0].Text = mode
			entered, release := make(chan struct{}), make(chan struct{})
			var unblock sync.Once
			releaseSink := func() { unblock.Do(func() { close(release) }) }
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
					if count == 1 {
						close(entered)
						<-release
					}
				})
				done <- outcome{result, err, count}
			}()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("sink did not start")
			}
			// This reply follows the whole burst on stdout. It must arrive while
			// the sink remains blocked, even at the maximum event budget.
			rpc, stop := context.WithTimeout(ctx, time.Second)
			defer stop()
			models, err := c.List(rpc)
			last := "4096"
			if mode == "overflow" {
				last = "4097"
			}
			if err != nil || len(models) != 1 || models[0].ModelID != last {
				t.Fatal("burst blocked concurrent RPC", models, err)
			}
			if mode == "cancel" {
				if err := c.Cancel(rpc, r.GenerationID); err != nil {
					t.Fatal("blocked sink prevented cancel acknowledgement", err)
				}
				cancel()
			}
			if mode == "close" {
				if err := c.Close(); err != nil {
					t.Fatal(err)
				}
			}
			releaseSink()
			select {
			case got := <-done:
				switch mode {
				case "cancel":
					if !errors.Is(got.err, llm.GenerationCancelled) || got.result.Text != "" {
						t.Fatal(got)
					}
				case "overflow":
					if !errors.Is(got.err, llm.InvalidProviderResponse) || got.result.Text != "" {
						t.Fatal(got)
					}
				default:
					if got.err != nil || got.result.Text != strings.Repeat("x", 4095) || got.count != 4096 {
						t.Fatal("valid mailbox lost events", got.count, got.err)
					}
				}
			case <-time.After(2 * time.Second):
				t.Fatal("generation did not finish after sink release")
			}
		})
	}
}
