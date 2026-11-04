package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/z-chenhao/assistant/internal/llm"
)

func recorderFixture(t *testing.T) (*Recorder, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; CREATE TABLE model_generation(generation_id TEXT PRIMARY KEY); INSERT INTO model_generation VALUES('generation'); CREATE TABLE run(root_run_id TEXT,created_at INTEGER,run_id TEXT);`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join("..", "..", "agent", "sqlite", "migrations", "031_dispatch_observations.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	r := NewRecorder(path)
	t.Cleanup(func() { r.Close() })
	if err := r.db.Ping(); err != nil {
		t.Fatal(err)
	}
	return r, db
}

func TestRecorderToleratesShortWriterContention(t *testing.T) {
	r, db := recorderFixture(t)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE model_generation SET generation_id=generation_id`); err != nil {
		t.Fatal(err)
	}
	released := make(chan error, 1)
	time.AfterFunc(60*time.Millisecond, func() { released <- tx.Commit() })
	r.Record(llm.GenerationRequest{GenerationID: "generation", OwnerToken: "owner"}, llm.Dispatch{GenerationID: "generation", Attempt: "attempt", Phase: "headers", At: time.Now().UnixMicro(), Format: "test", HTTPStatus: 200})
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-released; err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM dispatch_observation`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("short writer contention lost a completed HTTP observation: count=%d, error=%v", count, err)
	}
}

func TestProviderTimingsMigrationKeepsRecordedHTTPAttempt(t *testing.T) {
	r, db := recorderFixture(t)
	const body = `{"model":"fixture"}`
	if _, err := db.Exec(`INSERT INTO dispatch_observation VALUES
		('generation','attempt','started',1,'v1',?,'hash','',0,''),
		('generation','attempt','headers',2,'v1',NULL,'','',200,'')`, body); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join("..", "..", "agent", "sqlite", "migrations", "042_provider_timings.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	r.Record(llm.GenerationRequest{GenerationID: "generation", OwnerToken: "owner"}, llm.Dispatch{
		GenerationID: "generation", Attempt: "attempt", Phase: "first_body", At: 3, Format: "v1",
	})
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	var savedBody string
	var status, milestones int
	err = db.QueryRow(`SELECT
		(SELECT body_json FROM dispatch_observation WHERE phase='started'),
		(SELECT http_status FROM dispatch_observation WHERE phase='headers'),
		(SELECT count(*) FROM dispatch_observation WHERE phase='first_body' AND attempt='attempt' AND occurred_at=3)`).
		Scan(&savedBody, &status, &milestones)
	if err != nil || savedBody != body || status != 200 || milestones != 1 {
		t.Fatalf("upgrade lost HTTP evidence or rejected the new milestone: body=%q status=%d milestones=%d error=%v", savedBody, status, milestones, err)
	}
}

func TestRecorderDropsPersistentContentionWithoutBlockingCaller(t *testing.T) {
	r, db := recorderFixture(t)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE model_generation SET generation_id=generation_id`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		r.Record(llm.GenerationRequest{GenerationID: "generation", OwnerToken: "owner"}, llm.Dispatch{GenerationID: "generation", Attempt: "attempt", Phase: "headers", At: time.Now().UnixMicro(), Format: "test", HTTPStatus: 200})
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("diagnostics blocked the generation caller on a writer lock")
	}
	closed := make(chan error, 1)
	go func() { closed <- r.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("diagnostic write exceeded its bounded wait while the domain writer remained locked")
	}
}
