package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/collie/internal/agent"
	"github.com/z-chenhao/collie/internal/conversation"
	"github.com/z-chenhao/collie/internal/llm"
	"github.com/z-chenhao/collie/internal/memory"
	memorysqlite "github.com/z-chenhao/collie/internal/memory/sqlite"
	"github.com/z-chenhao/collie/internal/recall"
	"github.com/z-chenhao/collie/internal/recall/fake"
	"github.com/z-chenhao/collie/internal/tool"
)

func TestForgetRevokesFrozenPendingGeneration(t *testing.T) {
	ctx := context.Background()
	s, r, h, a, p := loopFixture(t)
	m, e := memorysqlite.Open(ctx, filepath.Dir(s.path))
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	if e = m.Configure(ctx, fake.Embedding{}); e != nil {
		t.Fatal(e)
	}
	if e = h.Configure(ctx, fake.Embedding{}); e != nil {
		t.Fatal(e)
	}
	registry, e := tool.NewRegistry([]tool.Definition{recall.Definition(fake.Embedding{}.Identity()), memory.Definition()})
	if e != nil {
		t.Fatal(e)
	}
	r.registry, a.Registry = registry, registry
	r.ConfigureRecall(recall.Service{Memory: m, Conversation: h})
	r.ConfigureMemory(m, memory.Vault{})
	receipt := mustSubmit(t, a, "before-forget", "tea context")
	c, e := r.Claim(ctx, receipt.RunID)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Initialize(ctx, c); e != nil {
		t.Fatal(e)
	}
	run, e := r.Load(ctx, c)
	if e != nil {
		t.Fatal(e)
	}
	inputs, e := r.Steering(ctx, c, run.Checkpoint.InputSequence)
	if e != nil {
		t.Fatal(e)
	}
	for _, input := range inputs {
		run.Checkpoint.Messages = append(run.Checkpoint.Messages, agent.AgentMessage{Type: "user", Content: input.Message.Content})
		run.Checkpoint.InputSequence = input.Sequence
	}
	if e = r.Checkpoint(ctx, c, run.Version, run.Checkpoint); e != nil {
		t.Fatal(e)
	}
	run, e = r.Load(ctx, c)
	if e != nil {
		t.Fatal(e)
	}
	const marker = "tea PRIVATE_REMEMBERED_MARKER"
	if e = m.Apply(ctx, memory.Record{ID: "victim", UserID: "local", ScopeType: "agent", ScopeID: run.AgentID, Tier: "recall", Content: marker, SourceRefs: []memory.SourceRef{{Type: "feedback", Ref: "feedback:original"}}, CreatedAt: time.Now()}); e != nil {
		t.Fatal(e)
	}
	gen, e := r.Generation(ctx, c, run)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := encoded(gen.Request.Messages)
	if !strings.Contains(raw, marker) {
		t.Fatal("fixture did not freeze fact")
	}
	first, e := r.ManageMemory(ctx, "forget-1", memory.Mutation{Operation: "forget", MemoryRef: "memory:victim"})
	if e != nil {
		t.Fatal(e)
	}
	again, e := r.ManageMemory(ctx, "forget-1", memory.Mutation{Operation: "forget", MemoryRef: "memory:victim"})
	if e != nil || first["receipt"] != again["receipt"] {
		t.Fatal(again, e)
	}
	if _, e = r.Generation(ctx, c, run); !errors.Is(e, agent.ErrClaimLost) {
		t.Fatal("forgotten frozen context reused", e)
	}
	if _, e = p.Generate(ctx, gen.Request, nil); !errors.Is(e, agent.ErrClaimLost) {
		t.Fatal("late acceptance succeeded", e)
	}
	if stopped, e := r.StopRequested(ctx, c); e != nil || !stopped {
		t.Fatal(stopped, e)
	}
	if settled, e := r.SettleStop(ctx, c); e != nil || !settled {
		t.Fatal(settled, e)
	}
	if e = r.CurateMemory(ctx); e != nil {
		t.Fatal(e)
	}
	hits, e := m.Search(ctx, recall.Scope{UserID: "local", AgentID: run.AgentID}, recall.Query{Query: "tea"})
	if e != nil {
		t.Fatal(e)
	}
	for _, hit := range hits {
		if strings.Contains(hit.Content, marker) {
			t.Fatal(hit)
		}
	}
	var frozen string
	if e = s.db.QueryRow(`SELECT hits_json FROM run_recall WHERE run_id=?`, run.ID).Scan(&frozen); e != nil || !strings.Contains(frozen, marker) {
		t.Fatal("audit lost", e)
	}
	if _, e = r.Claim(ctx, run.ID); !errors.Is(e, agent.ErrBusy) {
		t.Fatal("revoked run reclaimed", e)
	}
	if n := scalar(t, s, `SELECT count(*) FROM delivery_request`); n != 0 {
		t.Fatal("maintenance notified user", n)
	}
	newReceipt := mustSubmit(t, a, "after-forget", "tea again")
	next, e := r.Claim(ctx, newReceipt.RunID)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Release(ctx, next)
	if e = r.Initialize(ctx, next); e != nil {
		t.Fatal(e)
	}
	newRun, e := r.Load(ctx, next)
	if e != nil {
		t.Fatal(e)
	}
	if newRun.Snapshot.ModelID != run.Snapshot.ModelID {
		t.Fatal("model switched")
	}
}

func TestForgetPreservesFreshInputBeforeStopSettlement(t *testing.T) {
	for _, live := range []bool{false, true} {
		t.Run(map[bool]string{false: "queued", true: "live-owner"}[live], func(t *testing.T) {
			ctx := context.Background()
			s, r, _, a, p := loopFixture(t)
			m, err := memorysqlite.Open(ctx, filepath.Dir(s.path))
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			r.ConfigureMemory(m, memory.Vault{})
			old := mustSubmit(t, a, "old", "REVOKED_INPUT_MARKER")
			var owner agent.RunClaim
			if live {
				var err error
				owner, err = r.Claim(ctx, old.RunID)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := r.ManageMemory(ctx, "forget", memory.Mutation{Operation: "forget", Category: "health"}); err != nil {
				t.Fatal(err)
			}
			fresh := mustSubmit(t, a, "fresh", "FRESH_INPUT_MARKER")
			if fresh.RunID == old.RunID {
				t.Fatal("fresh input assigned to revoked Main")
			}
			if retry := mustSubmit(t, a, "fresh", "FRESH_INPUT_MARKER"); retry != fresh {
				t.Fatal("receipt changed", retry, fresh)
			}
			if _, err := a.Submit(ctx, "fresh", "different content"); !errors.Is(err, conversation.ErrConflict) {
				t.Fatal("fresh receipt accepted parameter drift", err)
			}
			if live {
				if _, err := r.Next(ctx); !errors.Is(err, agent.ErrBusy) {
					t.Fatal("successor scheduled before old owner joined", err)
				}
				if _, err := r.Claim(ctx, fresh.RunID); !errors.Is(err, agent.ErrBusy) {
					t.Fatal("successor claimed before old owner joined", err)
				}
				if stopped, err := r.SettleStop(ctx, owner); err != nil || !stopped {
					t.Fatal(stopped, err)
				}
			}
			mustDrain(t, a)
			if n := scalar(t, s, `SELECT count(*) FROM run WHERE run_id=? AND status='cancelled' AND error_code='memory_forgotten'`, old.RunID); n != 1 {
				t.Fatal("old Main revived")
			}
			if n := scalar(t, s, `SELECT count(*) FROM run WHERE run_id=? AND status='completed'`, fresh.RunID); n != 1 {
				t.Fatal("fresh Main stranded")
			}
			if len(p.requests) == 0 || scalar(t, s, `SELECT count(*) FROM delivery_request`) != 1 {
				t.Fatal("fresh input produced no answer")
			}
			for _, req := range p.requests {
				raw, _ := encoded(req)
				if strings.Contains(raw, "REVOKED_INPUT_MARKER") {
					t.Fatal("revoked context imported", raw)
				}
			}
			if retry := mustSubmit(t, a, "fresh", "FRESH_INPUT_MARKER"); retry != fresh {
				t.Fatal("terminal retry changed", retry, fresh)
			}
		})
	}
}

func TestForgetCanRevokeQueuedSuccessor(t *testing.T) {
	ctx := context.Background()
	s, r, _, a, p := loopFixture(t)
	m, err := memorysqlite.Open(ctx, filepath.Dir(s.path))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	r.ConfigureMemory(m, memory.Vault{})
	old := mustSubmit(t, a, "first", "REVOKED_FIRST_MARKER")
	owner, err := r.Claim(ctx, old.RunID)
	if err != nil {
		t.Fatal(err)
	}
	forget := func(key string) {
		t.Helper()
		if _, err := r.ManageMemory(ctx, key, memory.Mutation{Operation: "forget", Category: "health"}); err != nil {
			t.Fatal(err)
		}
	}
	forget("forget-first")
	middle := mustSubmit(t, a, "middle", "REVOKED_MIDDLE_MARKER")
	forget("forget-middle")
	fresh := mustSubmit(t, a, "last", "FRESH_LAST_MARKER")
	if old.RunID == middle.RunID || middle.RunID == fresh.RunID || old.RunID == fresh.RunID {
		t.Fatal("input ownership reused")
	}
	if next, err := r.Next(ctx); !errors.Is(err, agent.ErrBusy) {
		t.Fatal("last successor scheduled before first owner joined", next, err)
	}
	if stopped, err := r.SettleStop(ctx, owner); err != nil || !stopped {
		t.Fatal(stopped, err)
	}
	mustDrain(t, a)
	if scalar(t, s, `SELECT count(*) FROM run WHERE run_id IN (?,?) AND status='cancelled' AND error_code='memory_forgotten'`, old.RunID, middle.RunID) != 2 {
		t.Fatal("revoked predecessor revived")
	}
	if scalar(t, s, `SELECT count(*) FROM run WHERE run_id=? AND status='completed'`, fresh.RunID) != 1 {
		t.Fatal("last input stranded")
	}
	for _, req := range p.requests {
		raw, _ := encoded(req)
		if strings.Contains(raw, "REVOKED_") {
			t.Fatal("revoked predecessor imported", raw)
		}
	}
}

func TestContextLengthRetryKeepsSnapshotAndDoesNotReplayTools(t *testing.T) {
	s, r, _, a, p := loopFixture(t)
	p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
		if len(p.requests) == 1 {
			return llm.GenerationResult{}, llm.ContextLengthExceeded
		}
		return llm.GenerationResult{Text: "Recovered within the same frozen model"}, nil
	}
	receipt := mustSubmit(t, a, "context-retry", "answer using this model")
	mustDrain(t, a)
	if len(p.requests) != 2 || p.requests[0].Snapshot.SnapshotID != p.requests[1].Snapshot.SnapshotID || p.requests[0].GenerationID == p.requests[1].GenerationID {
		t.Fatal(p.requests)
	}
	if scalar(t, s, `SELECT count(*) FROM tool_invocation`) != 0 || scalar(t, s, `SELECT count(*) FROM model_generation WHERE status='failed' AND error_code='CONTEXT_LENGTH_EXCEEDED'`) != 1 {
		t.Fatal("retry duplicated effects or erased failure")
	}
	if _, e := r.Claim(context.Background(), receipt.RunID); !errors.Is(e, agent.ErrBusy) {
		t.Fatal("completed run resumed", e)
	}
}
