package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/llm"
)

func TestV18AcceptedContextReplaysWithoutRetrospectiveCompaction(t *testing.T) {
	ctx := context.Background()
	s, r, h, a, _ := loopFixture(t)
	receipt := mustSubmit(t, a, "legacy-context", "current request")
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
	for i := 0; i < 20; i++ {
		run.Checkpoint.Messages = append(run.Checkpoint.Messages, agent.TextMessage("assistant", strings.Repeat("old evidence ", 75)))
	}
	run.Checkpoint.Messages = append(run.Checkpoint.Messages, agent.TextMessage("user", "current request"))
	if e = r.Checkpoint(ctx, c, run.Version, run.Checkpoint); e != nil {
		t.Fatal(e)
	}
	run, e = r.Load(ctx, c)
	if e != nil {
		t.Fatal(e)
	}
	messages, e := agent.ProviderMessages(run.Checkpoint.Messages)
	if e != nil {
		t.Fatal(e)
	}
	old := llm.GenerationRequest{GenerationID: newID(), RunID: run.ID, SessionID: run.ID, Snapshot: run.Snapshot, Messages: messages, OutputLimitBytes: 64 << 10}
	for _, spec := range run.Tools {
		old.Tools = append(old.Tools, llm.ToolSpec{Name: spec.Name, Description: spec.Description, InputSchema: spec.InputSchema})
	}
	raw, e := encoded(old)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Exec(`INSERT INTO model_generation(generation_id,run_id,step_index,request_json,request_hash,owner_token,status,created_at) VALUES(?,?,?,?,?,?,'pending',?)`, old.GenerationID, run.ID, run.Checkpoint.Step, raw, digest(raw), c.(*claim).token, time.Now().UnixMicro()); e != nil {
		t.Fatal(e)
	}
	if e = r.Release(ctx, c); e != nil {
		t.Fatal(e)
	}
	path := s.path
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	// Remove only the new empty schema objects to reconstruct the populated v18
	// database. All model/Run/request records remain byte-for-byte unchanged.
	if _, e = db.Exec(`DROP TABLE computer_file; DROP TABLE computer_use; DROP TABLE computer_lease_resume; DROP TABLE computer_operation; DROP TABLE computer;
DROP TRIGGER run_memory_epoch;
DROP TABLE legacy_context_generation; DROP TABLE context_compaction; DROP TABLE run_context_policy;
DROP TABLE run_memory_epoch; DROP TABLE memory_privacy; DROP TABLE memory_mutation;
DROP TABLE skill_verification; DROP TABLE skill_candidate; PRAGMA user_version=18;`); e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := Open(ctx, filepath.Dir(path))
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	recovered := reopened.Runs(h, r.registry)
	c, e = recovered.Claim(ctx, run.ID)
	if e != nil {
		t.Fatal(e)
	}
	defer recovered.Release(ctx, c)
	run, e = recovered.Load(ctx, c)
	if e != nil {
		t.Fatal(e)
	}
	gen, e := recovered.Generation(ctx, c, run)
	if e != nil {
		t.Fatal(e)
	}
	if gen.Request.GenerationID != old.GenerationID || !reflect.DeepEqual(gen.Request.Messages, old.Messages) || gen.Request.Snapshot.SnapshotID != old.Snapshot.SnapshotID {
		t.Fatal("accepted legacy request changed")
	}
	var stored string
	if e = reopened.db.QueryRow(`SELECT request_json FROM model_generation WHERE generation_id=?`, old.GenerationID).Scan(&stored); e != nil || stored != raw {
		t.Fatal(e)
	}
}

func TestV19MainSuccessorMigrationRetainsInputAndStopOwnership(t *testing.T) {
	ctx := context.Background()
	s, r, history, a, _ := loopFixture(t)
	old := mustSubmit(t, a, "prior-input", "prior input")
	owner, err := r.Claim(ctx, old.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var original string
	if err = s.db.QueryRow(`SELECT message_json FROM main_input WHERE request_key='prior-input'`).Scan(&original); err != nil {
		t.Fatal(err)
	}
	// Schema 20 changes only this index. Reconstruct the real populated v19
	// schema while retaining its live owner, snapshots, input and privacy epoch.
	if _, err = s.db.Exec(`DROP TABLE computer_file; DROP TABLE computer_use; DROP TABLE computer_lease_resume; DROP TABLE computer_operation; DROP TABLE computer;
DROP INDEX one_main;
CREATE UNIQUE INDEX one_main ON run(agent_id) WHERE kind='main' AND status IN ('queued','running','waiting');
PRAGMA user_version=19;`); err != nil {
		t.Fatal(err)
	}
	path := s.path
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	recovered := reopened.Runs(history, r.registry)
	if scalar(t, reopened, `PRAGMA user_version`) != len(migrations) {
		t.Fatal("migration not applied")
	}
	var after string
	if err = reopened.db.QueryRow(`SELECT message_json FROM main_input WHERE request_key='prior-input'`).Scan(&after); err != nil || after != original {
		t.Fatal("input audit changed", err)
	}
	if scalar(t, reopened, `SELECT count(*) FROM run_claim WHERE run_id=? AND token=? AND epoch=?`, old.RunID, owner.(*claim).token, owner.(*claim).epoch) != 1 {
		t.Fatal("live owner changed")
	}
	agentID := idAgent(t, reopened)
	if err = recovered.write(ctx, func(tx *sql.Tx) error { return recovered.revokeMemory(ctx, tx, agentID) }); err != nil {
		t.Fatal(err)
	}
	fresh, err := recovered.Submit(ctx, "fresh-after-migration", []conversation.ContentPart{{Type: "text", Text: "fresh input"}})
	if err != nil || fresh.RunID == old.RunID {
		t.Fatal(fresh, err)
	}
	if _, err = recovered.Claim(ctx, fresh.RunID); !errors.Is(err, agent.ErrBusy) {
		t.Fatal("new owner admitted before join", err)
	}
	// Simulate an owner crash without wall-clock sleeps. Recovery observes the
	// expired persisted lease and must settle the predecessor before claiming.
	if _, err = reopened.db.Exec(`UPDATE run_claim SET expires_at=0,version=version+1 WHERE run_id=?`, old.RunID); err != nil {
		t.Fatal(err)
	}
	if next, err := recovered.Next(ctx); err != nil || next != fresh.RunID {
		t.Fatal(next, err)
	}
	if scalar(t, reopened, `SELECT count(*) FROM run WHERE run_id=? AND status='cancelled' AND error_code='memory_forgotten'`, old.RunID) != 1 {
		t.Fatal("expired predecessor not settled")
	}
	c, err := recovered.Claim(ctx, fresh.RunID)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Release(ctx, c)
}
