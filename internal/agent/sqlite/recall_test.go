package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/memory"
	memorysqlite "github.com/z-chenhao/erii/internal/memory/sqlite"
	"github.com/z-chenhao/erii/internal/recall"
	"github.com/z-chenhao/erii/internal/recall/fake"
	"github.com/z-chenhao/erii/internal/tool"
)

func TestRecallFrozenPerTurnAndFenced(t *testing.T) {
	ctx := context.Background()
	s, r, h, a, _ := loopFixture(t)
	m, e := memorysqlite.Open(ctx, filepath.Dir(s.path))
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	m.Configure(ctx, fake.Embedding{})
	h.Configure(ctx, fake.Embedding{})
	registry, e := tool.NewRegistry([]tool.Definition{recall.Definition(fake.Embedding{}.Identity())})
	if e != nil {
		t.Fatal(e)
	}
	r.registry = registry
	a.Registry = registry
	r.ConfigureRecall(recall.Service{Memory: m, Conversation: h})
	rc := mustSubmit(t, a, "recall", "tea context")
	c, e := r.Claim(ctx, rc.RunID)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Release(ctx, c)
	if e = r.Initialize(ctx, c); e != nil {
		t.Fatal(e)
	}
	run, e := r.Load(ctx, c)
	if e != nil {
		t.Fatal(e)
	}
	run.Checkpoint.Messages = append(run.Checkpoint.Messages, agent.TextMessage("user", "tea context"))
	if e = r.Checkpoint(ctx, c, run.Version, run.Checkpoint); e != nil {
		t.Fatal(e)
	}
	run, e = r.Load(ctx, c)
	if e != nil {
		t.Fatal(e)
	}
	first, e := r.Generation(ctx, c, run)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := encoded(first.Request.Messages)
	if e != nil || !strings.Contains(raw, "Retrieved durable context") {
		t.Fatal(raw, e)
	}
	if e = m.Apply(ctx, memory.Record{ID: "new-fact", UserID: "local", ScopeType: "agent", ScopeID: run.AgentID, Tier: "recall", Content: "tea NEW_AFTER_FREEZE", SourceRefs: []memory.SourceRef{{Type: "conversation", Ref: "conversation:fact"}}, CreatedAt: time.Now()}); e != nil {
		t.Fatal(e)
	}
	again, e := r.Generation(ctx, c, run)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(first.Request, again.Request) {
		t.Fatal("same turn changed after corpus mutation")
	}
	if strings.Contains(raw, "NEW_AFTER_FREEZE") {
		t.Fatal(raw)
	}
	var n int
	s.db.QueryRow(`SELECT count(*) FROM run_recall`).Scan(&n)
	if n != 1 {
		t.Fatal(n)
	}
	if _, e = s.db.Exec(`UPDATE run_recall SET hits_json='[]'`); e == nil {
		t.Fatal("mutable frozen recall")
	}
	// A new model turn independently recalls the newly committed fact.
	run.Checkpoint.Step++
	if e = r.Checkpoint(ctx, c, run.Version, run.Checkpoint); e != nil {
		t.Fatal(e)
	}
	run, e = r.Load(ctx, c)
	if e != nil {
		t.Fatal(e)
	}
	next, e := r.Generation(ctx, c, run)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ = encoded(next.Request.Messages)
	if !strings.Contains(raw, "NEW_AFTER_FREEZE") {
		t.Fatal("new turn failed to retrieve", raw)
	}
	// Static system prefix is byte stable; runtime evidence is a suffix.
	if !reflect.DeepEqual(first.Request.Messages[:2], next.Request.Messages[:2]) || next.Request.Messages[len(next.Request.Messages)-1].Origin != "runtime" {
		t.Fatal("cache prefix changed")
	}
	run.Checkpoint.Step++
	if e = r.Release(ctx, c); e != nil {
		t.Fatal(e)
	}
	if _, e = r.recallMessages(ctx, c, run, []llm.Message{{Role: "user", Text: "tea"}}); e != agent.ErrClaimLost {
		t.Fatal("old owner froze recall", e)
	}
}
func TestRecallFailureLeavesNoFrozenTurnOrModelCall(t *testing.T) {
	ctx := context.Background()
	s, r, h, a, _ := loopFixture(t)
	m, e := memorysqlite.Open(ctx, filepath.Dir(s.path))
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	m.Configure(ctx, fake.Embedding{Fail: true})
	h.Configure(ctx, fake.Embedding{})
	registry, _ := tool.NewRegistry([]tool.Definition{recall.Definition(fake.Embedding{}.Identity())})
	r.registry = registry
	a.Registry = registry
	r.ConfigureRecall(recall.Service{Memory: m, Conversation: h})
	receipt := mustSubmit(t, a, "fail-closed", "tea")
	c, e := r.Claim(ctx, receipt.RunID)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Release(ctx, c)
	if e = r.Initialize(ctx, c); e != nil {
		t.Fatal(e)
	}
	run, e := r.Load(ctx, c)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Generation(ctx, c, run); e != agent.ErrStorage {
		t.Fatal(e)
	}
	if scalar(t, s, `SELECT count(*) FROM run_recall`) != 0 || scalar(t, s, `SELECT count(*) FROM model_generation`) != 0 {
		t.Fatal("partial recall exposed")
	}
}

func TestRecallCapabilityDriftAndLegacyRun(t *testing.T) {
	registry, _ := tool.NewRegistry([]tool.Definition{recall.Definition("new-space")})
	r := &Runs{registry: registry, recall: &recall.Service{}}
	if id, e := r.recallIdentity(agent.Run{}); e != nil || id != "" {
		t.Fatal("legacy Run gained recall", id, e)
	}
	if _, e := r.recallIdentity(agent.Run{Tools: []tool.Spec{recall.Definition("old-space").Spec}}); e != llm.ModelIncompatible {
		t.Fatal("changed vector space accepted", e)
	}
	r.recall = nil
	if _, e := r.recallIdentity(agent.Run{Tools: []tool.Spec{recall.Definition("new-space").Spec}}); e != llm.ModelIncompatible {
		t.Fatal("removed provider accepted", e)
	}
}

func TestRecallMigrationFromPopulatedV17AndAtomicRollback(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	db, e := sql.Open("sqlite", filepath.Join(root, "state.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = migrate(ctx, db, migrations[:17]); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO agent_config(singleton,agent_id,setup_status,config_version,created_at,updated_at) VALUES(1,'prior-recall-agent','setup_required',7,1,1);
CREATE TRIGGER run_recall_immutable BEFORE UPDATE ON agent_config BEGIN SELECT 1; END;`); e != nil {
		t.Fatal(e)
	}
	if e = migrate(ctx, db, migrations); e == nil {
		t.Fatal("conflicting migration succeeded")
	}
	var version, tables int
	db.QueryRow(`PRAGMA user_version`).Scan(&version)
	db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name='run_recall'`).Scan(&tables)
	if version != 17 || tables != 0 {
		t.Fatal("partially committed migration", version, tables)
	}
	if _, e = db.Exec(`DROP TRIGGER run_recall_immutable`); e != nil {
		t.Fatal(e)
	}
	s, e := Open(ctx, root)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	d, e := s.Diagnostics(ctx)
	if e != nil || d.AgentID != "prior-recall-agent" || d.ConfigVersion != 7 || d.SchemaVersion != 20 {
		t.Fatal(d, e)
	}
}

type blockedRecall struct{ started chan struct{} }

func (b blockedRecall) Search(ctx context.Context, _ recall.Scope, _ recall.Query) ([]recall.Hit, error) {
	close(b.started)
	<-ctx.Done()
	return nil, ctx.Err()
}
func TestRecallCancelledWithLoopExecution(t *testing.T) {
	ctx := context.Background()
	s, r, h, a, _ := loopFixture(t)
	h.Configure(ctx, fake.Embedding{})
	registry, _ := tool.NewRegistry([]tool.Definition{recall.Definition("blocking-test")})
	r.registry = registry
	a.Registry = registry
	started := make(chan struct{})
	r.ConfigureRecall(recall.Service{Memory: blockedRecall{started}, Conversation: h})
	receipt := mustSubmit(t, a, "cancel-recall", "tea")
	c, e := r.Claim(ctx, receipt.RunID)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Release(ctx, c)
	if e = r.Initialize(ctx, c); e != nil {
		t.Fatal(e)
	}
	execution, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- agent.RunAgentLoop(ctx, c, agent.LoopOptions{Store: r, Execution: execution}) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("recall did not start")
	}
	cancel()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("cancelled recall succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("recall ignored execution cancellation")
	}
	if scalar(t, s, `SELECT count(*) FROM model_generation`) != 0 || scalar(t, s, `SELECT count(*) FROM run_recall`) != 0 {
		t.Fatal("cancelled evidence admitted")
	}
}
