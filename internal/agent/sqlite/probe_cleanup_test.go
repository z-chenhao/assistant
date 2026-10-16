package sqlite

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/llm/bridge"
)

// This real subprocess passes stream/tool/continuation checks, then stops
// acknowledging the probe's cancellation. It does not touch user state.
func TestProbeCleanupPeer(t *testing.T) {
	if len(os.Args) < 3 || os.Args[len(os.Args)-2] != "probe-cleanup-peer" {
		return
	}
	marker := os.Args[len(os.Args)-1]
	scanner := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var r struct {
			ID     uint64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &r) != nil {
			os.Exit(2)
		}
		reply := func(v any) {
			if enc.Encode(map[string]any{"jsonrpc": "2.0", "id": r.ID, "result": v}) != nil {
				os.Exit(2)
			}
		}
		switch r.Method {
		case "bridge.initialize":
			reply(map[string]int{"protocol_version": 1})
		case "bridge.shutdown":
			reply(struct{}{})
			os.Exit(0)
		case "models.list":
			reply([]llm.CatalogModel{{ModelID: strconv.Itoa(os.Getpid())}})
		case "generation.cancel":
			if os.WriteFile(marker, []byte("cancel received, deliberately unacknowledged"), 0600) != nil {
				os.Exit(2)
			}
		case "generation.start", "generation.continue":
			var req llm.GenerationRequest
			if json.Unmarshal(r.Params, &req) != nil {
				os.Exit(2)
			}
			reply(struct{}{})
			e := llm.GenerationEvent{GenerationID: req.GenerationID, Sequence: 1, Kind: "text_delta", Text: "OK"}
			last := req.Messages[len(req.Messages)-1]
			if r.Method == "generation.continue" {
				e.Text = last.Text
			} else if len(req.Tools) > 0 {
				e.Kind = "tool_call"
				e.Text = ""
				e.ToolCall = &llm.ToolCall{ID: "probe-call", Name: "probe_echo", Arguments: `{"value":"OK"}`}
			}
			enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "generation.event", "params": e})
			if !strings.Contains(last.Text, "until cancelled") {
				e = llm.GenerationEvent{GenerationID: req.GenerationID, Sequence: 2, Kind: "completed"}
				enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "generation.event", "params": e})
			}
		default:
			reply(struct{}{})
		}
	}
	os.Exit(0)
}

func TestProbeCleanupFailureClosesIngressAfterPeerReplacement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s := openModels(t)
	c := control(s)
	p, err := s.Models().AddProfile(ctx, modelInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Test(ctx, p.ProfileID); err != nil {
		t.Fatal(err)
	}
	if err = c.Select(ctx, p.ProfileID, 1, 1); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "cancel-observed")
	peer, err := bridge.Open(ctx, os.Args[0], "-test.run=^TestProbeCleanupPeer$", "--", "probe-cleanup-peer", marker)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	before, err := peer.List(ctx)
	if err != nil || len(before) != 1 {
		t.Fatal(before, err)
	}
	events := countRows(t, s, "event_outbox")
	c.Probe = peer
	_, err = c.Test(ctx, p.ProfileID)
	probeErr := err
	if !errors.Is(err, llm.GenerationCancelled) || !errors.Is(err, llm.ProviderUnavailable) || ctx.Err() != nil {
		t.Fatalf("unexpected probe failure %v caller=%v", err, ctx.Err())
	}
	if _, err = os.ReadFile(marker); err != nil {
		t.Fatal("no cancellation observed by real peer", err)
	}
	after, e := peer.List(ctx)
	if e != nil || len(after) != 1 {
		t.Fatal(after, e)
	}
	if before[0].ModelID == after[0].ModelID {
		t.Fatal("failed cleanup did not retire peer")
	}
	d, e := s.Diagnostics(ctx)
	if e != nil {
		t.Fatal(e)
	}
	effective := agent.EffectiveReadiness(ctx, d, c, peer)
	t.Logf("retired PID=%s replacement PID=%s; Test error=%v; caller=%v; Available=%v readiness=%s ingress=%v", before[0].ModelID, after[0].ModelID, probeErr, ctx.Err(), c.Available(ctx, p.ProfileID), effective.Readiness, effective.AgentIngressEnabled)
	if effective.AgentIngressEnabled || effective.Readiness != agent.Degraded || c.Available(ctx, p.ProfileID) || d.ConfigVersion != 3 {
		t.Fatal("failed cleanup retained ingress or config readiness", effective)
	}
	stored, e := s.Models().Profile(ctx, p.ProfileID)
	if e != nil || stored.Status != "unavailable" || stored.Version != 3 || stored.VerifiedAt != nil {
		t.Fatal("failed cleanup retained profile proof", stored, e)
	}
	if _, e = s.Models().Probe(ctx, p.ProfileID); !errors.Is(e, llm.ErrNotFound) {
		t.Fatal("failed cleanup retained probe record", e)
	}
	if countRows(t, s, "event_outbox") != events+2 {
		t.Fatal("missing profile/config invalidation events")
	}
	// A responsive replacement catalog is insufficient: a new successful Probe
	// and explicit version-bound selection are required to reopen ingress.
	c.Probe = probeFixture{result: passedProbe()}
	if _, e = c.Test(ctx, p.ProfileID); e != nil {
		t.Fatal(e)
	}
	if e = c.Select(ctx, p.ProfileID, 3, 3); e != nil {
		t.Fatal(e)
	}
	d, e = s.Diagnostics(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if effective = agent.EffectiveReadiness(ctx, d, c, peer); !effective.AgentIngressEnabled {
		t.Fatal("fresh probe and selection could not recover", effective)
	}
}
