package bridge_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/collie/internal/llm"
	"github.com/z-chenhao/collie/internal/llm/bridge"
)

// TestTransportProcess is a disposable peer with observable transport faults.
// Its finite stall also lets the pre-fix regression exit instead of hanging CI.
func TestTransportProcess(t *testing.T) {
	if len(os.Args) < 3 || !strings.HasPrefix(os.Args[len(os.Args)-2], "transport:") {
		return
	}
	mode := strings.TrimPrefix(os.Args[len(os.Args)-2], "transport:")
	marker := os.Args[len(os.Args)-1]
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	type rpc struct {
		ID     uint64          `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	reply := func(id uint64, value any) {
		if encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": value}) != nil {
			os.Exit(2)
		}
	}
	var hello rpc
	if decoder.Decode(&hello) != nil || hello.Method != "bridge.initialize" {
		os.Exit(2)
	}
	if mode == "restart" {
		if _, err := os.Stat(marker); err == nil {
			if os.WriteFile(marker+".restarting", []byte("initializing"), 0600) != nil {
				os.Exit(2)
			}
			_, _ = io.Copy(io.Discard, os.Stdin) // no handshake response until Host terminates us
			os.Exit(0)
		}
		if os.WriteFile(marker, []byte("initial start"), 0600) != nil {
			os.Exit(2)
		}
	}
	reply(hello.ID, map[string]int{"protocol_version": bridge.ProtocolVersion})
	if mode == "stall" {
		var first [1]byte
		if _, err := os.Stdin.Read(first[:]); err != nil {
			os.Exit(2)
		}
		if os.WriteFile(marker, []byte("writer dispatched"), 0600) != nil {
			os.Exit(2)
		}
		time.Sleep(3 * time.Second)
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	var startID uint64
	var barriers []uint64
	state := "idle"
	for {
		var r rpc
		if decoder.Decode(&r) != nil {
			os.Exit(0)
		}
		switch r.Method {
		case "generation.start", "generation.continue":
			if mode == "restart" {
				os.Exit(0)
			}
			ledger, err := os.OpenFile(marker+".generations", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				os.Exit(2)
			}
			if _, err = ledger.WriteString(r.Method + "\n"); err != nil {
				os.Exit(2)
			}
			if ledger.Close() != nil {
				os.Exit(2)
			}
			startID = r.ID
			state = "started"
			for _, id := range barriers {
				reply(id, llm.AuthState{Status: state})
			}
			barriers = nil
			// Deliberately withhold the start ack until cancellation is received.
		case "auth.status":
			if startID == 0 {
				barriers = append(barriers, r.ID)
			} else {
				reply(r.ID, llm.AuthState{Status: state})
			}
		case "generation.cancel":
			if mode == "cancel-stalls" {
				continue
			}
			var p struct {
				GenerationID string `json:"generation_id"`
			}
			if json.Unmarshal(r.Params, &p) != nil {
				os.Exit(2)
			}
			state = "cancelled:" + p.GenerationID
			reply(r.ID, struct{}{})
			reply(startID, struct{}{}) // late acknowledgement must not revive it
		case "bridge.shutdown":
			reply(r.ID, struct{}{})
			os.Exit(0)
		case "models.list":
			reply(r.ID, []llm.CatalogModel{{ModelID: strconv.Itoa(os.Getpid())}})
		default:
			reply(r.ID, struct{}{})
		}
	}
}

func TestRestartAdmissionAndCloseDuringHandshake(t *testing.T) {
	for _, operation := range []string{"admission", "close", "concurrent-close"} {
		t.Run(operation, func(t *testing.T) {
			c, marker := transport(t, "restart")
			if _, err := c.Generate(context.Background(), request(), nil); !errors.Is(err, llm.ProviderUnavailable) {
				t.Fatal("peer did not crash", err)
			}
			restart := make(chan error, 1)
			go func() { _, err := c.Status(context.Background()); restart <- err }()
			waitForDispatch(t, marker+".restarting")
			if operation == "admission" {
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				start := time.Now()
				_, err := c.Status(ctx)
				cancel()
				if elapsed := time.Since(start); err == nil || elapsed > 250*time.Millisecond {
					t.Errorf("restart admission ignored caller deadline: %s %v", elapsed, err)
				}
			}
			start := time.Now()
			if operation == "concurrent-close" {
				closed := make(chan error, 2)
				for range 2 {
					go func() { closed <- c.Close() }()
				}
				for range 2 {
					select {
					case err := <-closed:
						if err != nil {
							t.Fatal(err)
						}
					case <-time.After(time.Second):
						t.Fatal("concurrent Close remained blocked during initialization")
					}
				}
			} else if err := c.Close(); err != nil {
				t.Fatal(err)
			}
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Errorf("Close could not interrupt initialization: %s", elapsed)
			}
			select {
			case err := <-restart:
				if err == nil {
					t.Error("closed initializer returned success")
				}
			case <-time.After(time.Second):
				t.Fatal("initializer remained active after Close")
			}
		})
	}
}

func TestInternalAcknowledgementTimeoutCleansGeneration(t *testing.T) {
	for _, mode := range []string{"start", "continue", "cancel-stalls"} {
		t.Run(mode, func(t *testing.T) {
			fixture := "delayed-ack"
			if mode == "cancel-stalls" {
				fixture = mode
			}
			c, marker := transport(t, fixture)
			before, err := c.List(context.Background())
			if err != nil || len(before) != 1 {
				t.Fatal("peer identity unavailable", before, err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if mode == "continue" {
				_, err = c.Continue(ctx, request(), nil)
			} else {
				_, err = c.Generate(ctx, request(), nil)
			}
			if !errors.Is(err, llm.ProviderUnavailable) || errors.Is(err, llm.GenerationCancelled) || ctx.Err() != nil {
				t.Fatalf("internal timeout changed caller cancellation/error contract: %v caller=%v", err, ctx.Err())
			}
			if mode == "cancel-stalls" {
				after, err := c.List(ctx)
				if err != nil || len(after) != 1 || after[0].ModelID == before[0].ModelID {
					t.Fatal("unresponsive cancellation did not retire Bridge", after, err)
				}
			} else {
				state, err := c.Status(ctx)
				if err != nil || state.Status != "cancelled:generation" {
					t.Fatal("timed-out generation retained by peer", state, err)
				}
			}
			calls, err := os.ReadFile(marker + ".generations")
			if err != nil || strings.Count(string(calls), "\n") != 1 {
				t.Fatalf("generation replayed: %q %v", calls, err)
			}
		})
	}
}

func transport(t *testing.T, mode string) (*bridge.Client, string) {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "dispatched")
	c, err := bridge.Open(context.Background(), os.Args[0], "-test.run=^TestTransportProcess$", "--", "transport:"+mode, marker)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, marker
}

func waitForDispatch(t *testing.T, marker string) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("peer did not observe request dispatch")
		case <-ticker.C:
			if _, err := os.Stat(marker); err == nil {
				return
			}
		}
	}
}

func TestStalledReaderWriteDeadline(t *testing.T) {
	c, _ := transport(t, "stall")
	r := request()
	r.Messages[0].Text = strings.Repeat("x", 512<<10)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Generate(ctx, r, nil)
	elapsed := time.Since(start)
	if elapsed > time.Second {
		t.Errorf("blocked write exceeded cancellation bound: %s", elapsed)
	}
	if !errors.Is(err, llm.GenerationCancelled) {
		t.Errorf("cancelled write returned %v", err)
	}
}

func TestStalledReaderAdmissionAndClose(t *testing.T) {
	for _, operation := range []string{"admission", "close"} {
		t.Run(operation, func(t *testing.T) {
			c, marker := transport(t, "stall")
			r := request()
			r.Messages[0].Text = strings.Repeat("x", 512<<10)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := c.Generate(ctx, r, nil); done <- err }()
			waitForDispatch(t, marker)
			if operation == "admission" {
				queuedCtx, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
				start := time.Now()
				_, err := c.Status(queuedCtx)
				stop()
				if elapsed := time.Since(start); elapsed > time.Second {
					t.Errorf("writer admission ignored deadline: %s", elapsed)
				}
				if err == nil {
					t.Error("queued RPC unexpectedly succeeded")
				}
			}
			start := time.Now()
			if err := c.Close(); err != nil {
				t.Error(err)
			}
			if elapsed := time.Since(start); elapsed > 2*time.Second {
				t.Errorf("Close waited for blocked writer: %s", elapsed)
			}
			select {
			case err := <-done:
				if err == nil {
					t.Error("blocked generation unexpectedly completed")
				}
			case <-time.After(time.Second):
				t.Fatal("Close did not release blocked generation")
			}
		})
	}
}

func TestCancellationBeforeStartAcknowledgement(t *testing.T) {
	for _, tc := range []struct{ continuation, deadline bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
		name := "start"
		if tc.continuation {
			name = "continue"
		}
		if tc.deadline {
			name += "-deadline"
		}
		t.Run(name, func(t *testing.T) {
			c, _ := transport(t, "delayed-ack")
			ctx, cancel := context.WithCancel(context.Background())
			if tc.deadline {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 150*time.Millisecond)
			}
			defer cancel()
			done := make(chan error, 1)
			go func() {
				var err error
				if tc.continuation {
					_, err = c.Continue(ctx, request(), nil)
				} else {
					_, err = c.Generate(ctx, request(), nil)
				}
				done <- err
			}()
			barrier, stop := context.WithTimeout(context.Background(), 2*time.Second)
			defer stop()
			state, err := c.Status(barrier)
			if err != nil || state.Status != "started" {
				t.Fatal("generation not dispatched", state, err)
			}
			if tc.deadline {
				<-ctx.Done()
			} else {
				cancel()
			}
			select {
			case err := <-done:
				if !errors.Is(err, llm.GenerationCancelled) {
					t.Errorf("cancel before ack returned %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("cancel before ack hung")
			}
			state, err = c.Status(barrier)
			if err != nil || state.Status != "cancelled:generation" {
				t.Fatalf("peer retained generation: %+v %v", state, err)
			}
		})
	}
}
