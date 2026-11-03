package sqlite

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/task"
	"github.com/z-chenhao/assistant/internal/tool"
)

func TestMainRespondsBeforeSlowToolFinishes(t *testing.T) {
	testMainRespondsBeforeSlowToolFinishes(t, false)
}

func TestMainTaskControlRespondsWithFullToolCapacity(t *testing.T) {
	testMainRespondsBeforeSlowToolFinishes(t, true)
}

func testMainRespondsBeforeSlowToolFinishes(t *testing.T, saturated bool) {
	t.Helper()
	s, _, h, a, p := loopFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	definition, _ := a.Registry.Resolve("mcp__fixture__echo")
	definition.Execute = func(ctx context.Context, _ string) tool.Result {
		once.Do(func() { close(started) })
		select {
		case <-release:
			return tool.Result{Text: "slow result"}
		case <-ctx.Done():
			return tool.Failure("CANCELLED")
		}
	}
	definitions := []tool.Definition{definition}
	if saturated {
		definitions = append(definitions, task.Definitions()...)
	}
	registry, err := tool.NewRegistry(definitions)
	if err != nil {
		t.Fatal(err)
	}
	a.Registry = registry
	runs := s.Runs(h, registry)
	if saturated {
		runs.ConfigureToolLimits(ToolLimits{Run: 1})
	}
	a.Store = runs
	p.respond = func(request llm.GenerationRequest) (llm.GenerationResult, error) {
		last := request.Messages[len(request.Messages)-1]
		switch {
		case last.Role == "user" && last.Origin != "runtime" && last.Text == "long work":
			return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "slow", Name: definition.Spec.Name, Arguments: `{"text":"long"}`}}}, nil
		case last.Role == "user" && last.Origin != "runtime" && last.Text == "ping":
			if saturated {
				return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "search", Name: task.SearchName, Arguments: `{}`}}}, nil
			}
			return llm.GenerationResult{Text: "pong"}, nil
		case last.Role == "tool" && last.ToolCallID == "search":
			return llm.GenerationResult{Text: "pong"}, nil
		case last.Role == "tool" || last.Origin == "runtime":
			return llm.GenerationResult{Text: "original finished"}, nil
		default:
			return llm.GenerationResult{}, llm.InvalidProviderResponse
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	mustSubmit(t, a, "long", "long work")
	go func() { done <- a.Drain(ctx) }()
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("Main did not join after cancellation")
		}
	})
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("slow tool did not start")
	}
	mustSubmit(t, a, "ping", "ping")
	deadline := time.Now().Add(3 * time.Second)
	for {
		messages, e := h.List(ctx, idAgent(t, s))
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, m := range messages {
			for _, part := range m.Content {
				found = found || m.Role == "assistant" && part.Text == "pong"
			}
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("new input remained blocked behind the unfinished tool")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if scalar(t, s, `SELECT count(*) FROM run WHERE kind='main'`) != 1 || scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE status='running'`) != 1 {
		t.Fatal("response used a second Main or lost the active tool")
	}
	close(release)
	select {
	case err := <-done:
		// Leave the result available for cleanup too.
		done <- err
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Main did not deliver the deferred result")
	}
	want := 1
	if saturated {
		want = 2
	}
	if scalar(t, s, `SELECT count(*) FROM tool_invocation`) != want || scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE status='succeeded' AND attempt_count=1`) != want {
		t.Fatal("tool was repeated")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	foundRuntime := false
	for _, request := range p.requests {
		for _, message := range request.Messages {
			foundRuntime = foundRuntime || message.Origin == "runtime" && strings.Contains(message.Text, "slow result")
		}
	}
	if !foundRuntime {
		for _, request := range p.requests {
			for _, message := range request.Messages {
				if message.Role != "system" {
					t.Logf("role=%s origin=%s text=%s", message.Role, message.Origin, message.Text)
				}
			}
		}
		var checkpoint string
		_ = s.db.QueryRow(`SELECT checkpoint_json FROM run WHERE kind='main'`).Scan(&checkpoint)
		t.Fatalf("deferred result lost its runtime provenance: %s", checkpoint)
	}
}

func TestMainPendingApprovalAcceptsSteeringWithoutDispatch(t *testing.T) {
	s, _, h, a, p := loopFixture(t)
	definition := effectDef(t, "never")
	registry, err := tool.NewRegistry([]tool.Definition{definition})
	if err != nil {
		t.Fatal(err)
	}
	a.Registry = registry
	r := s.Runs(h, registry)
	a.Store = r
	p.respond = func(request llm.GenerationRequest) (llm.GenerationResult, error) {
		last := request.Messages[len(request.Messages)-1]
		if last.Role == "user" && last.Origin != "runtime" && last.Text == "effect" {
			return llm.GenerationResult{ToolCalls: effectCall(`{"target":"fixture","text":"not sent"}`)}, nil
		}
		return llm.GenerationResult{Text: "steering handled"}, nil
	}
	first := mustSubmit(t, a, "effect", "effect")
	mustDrain(t, a)
	if scalar(t, s, `SELECT count(*) FROM run WHERE status='waiting' AND wait_reason='approval'`) != 1 {
		t.Fatal("effect did not wait for approval")
	}
	second := mustSubmit(t, a, "correction", "change the request")
	if second.RunID != first.RunID {
		t.Fatal("approval steering opened another Main")
	}
	mustDrain(t, a)
	if scalar(t, s, `SELECT count(*) FROM approval WHERE status='invalidated'`) != 1 || scalar(t, s, `SELECT sum(attempt_count) FROM tool_invocation`) != 0 || scalar(t, s, `SELECT count(*) FROM run WHERE status='completed'`) != 1 {
		t.Fatal("steering reused approval, dispatched the effect, or failed to finish")
	}
}

func TestMainSpendWaitDrainsAcceptedSlowTool(t *testing.T) {
	s, _, h, a, p := loopFixture(t)
	started, release, cancelled := make(chan struct{}), make(chan struct{}), make(chan struct{})
	definition, _ := a.Registry.Resolve("mcp__fixture__echo")
	definition.Execute = func(ctx context.Context, _ string) tool.Result {
		close(started)
		select {
		case <-release:
			return tool.Result{Text: "accepted outcome"}
		case <-ctx.Done():
			close(cancelled)
			return tool.Failure("CANCELLED")
		}
	}
	registry, err := tool.NewRegistry([]tool.Definition{definition})
	if err != nil {
		t.Fatal(err)
	}
	r := s.Runs(h, registry)
	r.ConfigureSpend(spendPolicy(t, 12000))
	a.Store, a.Registry = r, registry
	p.respond = func(request llm.GenerationRequest) (llm.GenerationResult, error) {
		last := request.Messages[len(request.Messages)-1]
		if last.Text == "long work" {
			return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "slow", Name: definition.Spec.Name, Arguments: `{"text":"long"}`}}, Usage: &llm.TokenUsage{InputTokens: 2000}}, nil
		}
		return llm.GenerationResult{Text: "received"}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("Main did not drain accepted tool")
		}
	})
	mustSubmit(t, a, "priced-long", "long work")
	go func() { done <- a.Drain(ctx) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("slow tool did not start")
	}
	mustSubmit(t, a, "priced-ping", "ping")
	deadline := time.Now().Add(3 * time.Second)
	for scalar(t, s, `SELECT count(*) FROM spend_approval WHERE status='pending'`) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("next model step did not reach spend consent")
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case <-cancelled:
		t.Fatal("model spend wait cancelled an already accepted tool")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		done <- err
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("spend wait did not join accepted tool")
	}
	if scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE status='succeeded' AND attempt_count=1`) != 1 {
		t.Fatal("spend wait lost the accepted outcome")
	}
	approvals, err := r.Approvals(ctx)
	if err != nil || len(approvals) != 1 {
		t.Fatal(approvals, err)
	}
	if err = r.Decide(ctx, idAgent(t, s), approvals[0].ID, approvals[0].Hash, "approved"); err != nil {
		t.Fatal(err)
	}
	mustDrain(t, a)
	if scalar(t, s, `SELECT count(*) FROM run WHERE status='completed'`) != 1 || scalar(t, s, `SELECT attempt_count FROM tool_invocation`) != 1 {
		t.Fatal("consent resume repeated or abandoned the accepted call")
	}
}

func TestDeferredToolRecoveryRetainsOutcomeAndAuthority(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		name := "completed"
		if unknown {
			name = "unknown"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			definition, _ := tool.FixtureRegistry().Resolve("mcp__fixture__echo")
			calls := []llm.ToolCall{{ID: "original", Name: definition.Spec.Name, Arguments: `{"text":"durable result"}`}}
			if unknown {
				definition = effectDef(t, "never")
				calls = effectCall(`{"target":"fixture","text":"uncertain","behavior":"unknown"}`)
			}
			s, r, claim, executor, exchange := admitPolicy(t, []tool.Definition{definition}, calls)
			if unknown {
				if _, err := executor.Execute(ctx, exchange, calls); err != tool.ErrSuspended {
					t.Fatal(err)
				}
				approvePolicy(t, s, r)
			}
			if _, err := executor.Execute(ctx, exchange, calls); err != nil {
				t.Fatal(err)
			}
			run, err := r.Load(ctx, claim)
			if err != nil {
				t.Fatal(err)
			}
			cp := run.Checkpoint
			cp.PendingTools = []agent.PendingToolBatch{{ExchangeID: exchange, Step: 0, Calls: calls}}
			cp.Step, cp.Phase = 1, agent.ModelPhase
			if err = r.Checkpoint(ctx, claim, run.Version, cp); err != nil {
				t.Fatal(err)
			}
			if err = r.Release(ctx, claim); err != nil {
				t.Fatal(err)
			}
			// A fresh owner reconstructs pending delivery from durable records.
			p := &loopProvider{store: s, respond: func(request llm.GenerationRequest) (llm.GenerationResult, error) {
				last := request.Messages[len(request.Messages)-1]
				if last.Origin != "runtime" || !strings.Contains(last.Text, "operation:0:0") || unknown && !strings.Contains(last.Text, "UNKNOWN_SIDE_EFFECT") || !unknown && !strings.Contains(last.Text, "durable result") {
					return llm.GenerationResult{}, llm.InvalidProviderResponse
				}
				return llm.GenerationResult{Text: "outcome received"}, nil
			}}
			a := agent.Actor{Store: r, Provider: p, Registry: r.registry, Ready: func(context.Context) bool { return true }}
			mustDrain(t, a)
			mustDrain(t, a)
			if scalar(t, s, `SELECT count(*) FROM tool_invocation`) != 1 || scalar(t, s, `SELECT attempt_count FROM tool_invocation`) != 1 || scalar(t, s, `SELECT count(*) FROM delivery_request d JOIN main_reply r USING(delivery_id)`) != 1 {
				t.Fatal("recovery repeated a call or delivery")
			}
			if unknown && scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE status='unknown'`) != 1 {
				t.Fatal("unknown effect was retried or declared failed")
			}
		})
	}
}

func TestMainSpendDenialRecoversAcceptedUnknownTool(t *testing.T) {
	ctx := context.Background()
	s, _, h, a, p := loopFixture(t)
	registry, err := tool.NewRegistry([]tool.Definition{effectDef(t, "never")})
	if err != nil {
		t.Fatal(err)
	}
	r := s.Runs(h, registry)
	r.ConfigureSpend(spendPolicy(t, 12000))
	a.Store, a.Registry = r, registry
	calls := effectCall(`{"target":"fixture","text":"uncertain"}`)
	p.respond = func(llm.GenerationRequest) (llm.GenerationResult, error) {
		return llm.GenerationResult{ToolCalls: calls, Usage: &llm.TokenUsage{InputTokens: 2000}}, nil
	}
	receipt := mustSubmit(t, a, "priced-recovery", "long work")
	claim, err := r.Claim(ctx, receipt.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Initialize(ctx, claim); err != nil {
		t.Fatal(err)
	}
	run, err := r.Load(ctx, claim)
	if err != nil {
		t.Fatal(err)
	}
	generation, err := r.Generation(ctx, claim, run)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Generate(ctx, generation.Request, nil); err != nil {
		t.Fatal(err)
	}
	store := r.ToolStore(claim)
	invs, err := store.Accept(ctx, generation.Request.GenerationID, calls, registry.Describe())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Prepare(ctx, invs[0].ID); err != nil {
		t.Fatal(err)
	}
	approvePolicy(t, s, r)
	if err = store.Start(ctx, invs[0].ID); err != nil {
		t.Fatal(err)
	}
	// A process died after dispatch and durable deferral, before the outcome.
	cp := run.Checkpoint
	cp.PendingTools = []agent.PendingToolBatch{{ExchangeID: generation.Request.GenerationID, Step: 0, Calls: calls}}
	cp.Step, cp.Phase = 1, agent.ModelPhase
	if err = r.Checkpoint(ctx, claim, run.Version, cp); err != nil {
		t.Fatal(err)
	}
	run, err = r.Load(ctx, claim)
	if err != nil {
		t.Fatal(err)
	}
	generation, err = r.Generation(ctx, claim, run)
	if err != nil || !generation.Waiting {
		t.Fatal("next model step did not require consent", generation, err)
	}
	if err = r.Wait(ctx, claim, run.Version, "resource"); err != nil {
		t.Fatal(err)
	}
	approvals, err := r.Approvals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, approval := range approvals {
		if approval.Status == "pending" {
			if err = r.Decide(ctx, run.AgentID, approval.ID, approval.Hash, "denied"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = a.Drain(ctx); !errors.Is(err, agent.ErrSpendDenied) {
		t.Fatal("denied Main did not fail", err)
	}
	if scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE status='unknown' AND attempt_count=1`) != 1 {
		t.Fatal("denied recovery orphaned or retried the accepted effect")
	}
}

func TestResponsiveProtocolMigrationIsAtomic(t *testing.T) {
	ctx := context.Background()
	db := schemaDB(t)
	if err := migrate(ctx, db, migrations[:31]); err != nil {
		t.Fatal(err)
	}
	broken := append(append([]string(nil), migrations[:31]...), responsiveMainSQL+"; INVALID SQL")
	if err := migrate(ctx, db, broken); err == nil {
		t.Fatal("partial protocol migration was accepted")
	}
	var version, tables int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name='main_tool_protocol'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if version != 31 || tables != 0 {
		t.Fatal("failed migration left protocol metadata behind", version, tables)
	}
	if err := migrate(ctx, db, migrations); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, db, migrations[:31]); err == nil {
		t.Fatal("older schema accepted asynchronous checkpoints")
	}
	s, _, _, a, _ := loopFixture(t)
	receipt := mustSubmit(t, a, "protocol", "a new Main")
	if scalar(t, s, `SELECT count(*) FROM main_tool_protocol WHERE run_id=?`, receipt.RunID) != 1 {
		t.Fatal("new Main lacks frozen protocol metadata")
	}
	for _, query := range []string{`DELETE FROM main_tool_protocol WHERE run_id=?`, `UPDATE main_tool_protocol SET version=1 WHERE run_id=?`} {
		if _, err := s.db.Exec(query, receipt.RunID); err == nil {
			t.Fatal("Run protocol was mutable")
		}
	}
}
