package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/z-chenhao/collie/internal/llm/memory"
	mem "github.com/z-chenhao/collie/internal/memory"
	"github.com/z-chenhao/collie/internal/recall"
	"github.com/z-chenhao/collie/internal/recall/fake"
	"github.com/z-chenhao/collie/internal/recall/sqliteindex"
)

func TestCuratorEvidenceDedupEncryptionAndForget(t *testing.T) {
	s := openTest(t)
	secrets := memory.New()
	if e := secrets.Put(ctx, "memory-key", bytes.Repeat([]byte{7}, 32)); e != nil {
		t.Fatal(e)
	}
	s.ConfigureVault(mem.Vault{Secrets: secrets, Ref: "memory-key"})
	for i, event := range []string{"one", "two", "three"} {
		ref := mem.SourceRef{Type: "conversation", Ref: "conversation:" + event}
		if e := s.Curate(ctx, event, scope, ref, time.Unix(int64(101+i), 0), []string{"I prefer green tea"}, false, ""); e != nil {
			t.Fatal(e)
		}
		if e := s.Curate(ctx, event, scope, ref, time.Unix(int64(101+i), 0), []string{"I prefer green tea"}, false, ""); e != nil {
			t.Fatal(e)
		}
	}
	var count int
	if s.db.QueryRow(`SELECT count(*) FROM memory_record`).Scan(&count) != nil || count != 2 {
		t.Fatal("dedup/promotion", count)
	}
	var tier string
	if s.db.QueryRow(`SELECT tier FROM memory_record WHERE memory_id='three-0'`).Scan(&tier) != nil || tier != "core" {
		t.Fatal(tier)
	}
	const health = "My health report: morning fatigue after late sleep, not a diagnosis"
	if e := s.Curate(ctx, "health", scope, mem.SourceRef{Type: "conversation", Ref: "conversation:health"}, time.Unix(105, 0), []string{health}, false, ""); e != nil {
		t.Fatal(e)
	}
	hits, e := s.Search(ctx, scope, recall.Query{Query: "fatigue"})
	if e != nil || len(hits) != 1 || hits[0].Content != health {
		t.Fatal(hits, e)
	}
	if e = s.Curate(ctx, "health", scope, mem.SourceRef{Type: "conversation", Ref: "conversation:health"}, time.Unix(105, 0), []string{"different"}, false, ""); e != recall.ErrConflict {
		t.Fatal("event drift", e)
	}
	for _, name := range []string{"memory.db", "memory.db-wal"} {
		data, e := os.ReadFile(filepath.Join(filepath.Dir(s.dbPath(t)), name))
		if e != nil && !os.IsNotExist(e) {
			t.Fatal(e)
		}
		if bytes.Contains(data, []byte(health)) {
			t.Fatal("sensitive plaintext at rest")
		}
	}
	if e = s.Forget(ctx, scope, "three-0", time.Unix(110, 0)); e != nil {
		t.Fatal(e)
	}
	if e = s.Curate(ctx, "old-source-retry", scope, mem.SourceRef{Type: "conversation", Ref: "conversation:three"}, time.Unix(111, 0), []string{"I prefer green tea"}, false, ""); e != nil {
		t.Fatal(e)
	}
	hits, e = s.Search(ctx, scope, recall.Query{Query: "tea"})
	if e != nil {
		t.Fatal(e)
	}
	for _, h := range hits {
		if h.Content == "I prefer green tea" {
			t.Fatal("suppressed source resurrected")
		}
	}
}

func TestFreshRememberRetainsIndependentProvenance(t *testing.T) {
	s := openTest(t)
	const text = "I prefer unsweetened tea"
	curate := func(id, source string, at int64) {
		t.Helper()
		if err := s.Curate(ctx, id, scope, mem.SourceRef{Type: "feedback", Ref: "feedback:" + source}, time.Unix(at, 0), []string{text}, true, ""); err != nil {
			t.Fatal(err)
		}
	}
	curate("original", "original", 100)
	floor := time.Unix(110, 0)
	query := recall.Query{Query: "tea", TimeRange: &recall.TimeRange{Start: &floor}}
	if hits, err := s.Search(ctx, scope, query); err != nil || len(hits) != 0 {
		t.Fatal("old evidence crossed privacy floor", hits, err)
	}
	curate("fresh", "fresh", 120)
	curate("fresh", "fresh", 120)       // receipt replay
	curate("same-source", "fresh", 130) // no independent evidence
	curate("late-old", "late-old", 90)  // out of order cannot replace fresh provenance
	hits, err := s.Search(ctx, scope, query)
	if err != nil || len(hits) != 1 || hits[0].Content != text || hits[0].SourceRef != "memory:fresh-0" {
		t.Fatal("fresh remember invisible", hits, err)
	}
	var refs, successor string
	var created, count int64
	if err = s.db.QueryRow(`SELECT source_refs,supersedes_memory_id,created_at FROM memory_record WHERE memory_id='fresh-0'`).Scan(&refs, &successor, &created); err != nil {
		t.Fatal(err)
	}
	if successor != "original-0" || created != time.Unix(120, 0).UnixMicro() || refs != `[{"source_ref":"feedback:fresh","source_type":"feedback"}]` {
		t.Fatal("fresh provenance changed", refs, successor, created)
	}
	if err = s.db.QueryRow(`SELECT count(*) FROM memory_record`).Scan(&count); err != nil || count != 2 {
		t.Fatal("replay/duplicate created revisions", count, err)
	}
	if err = s.Forget(ctx, scope, "fresh-0", time.Unix(140, 0)); err != nil {
		t.Fatal(err)
	}
	curate("suppressed", "fresh", 150)
	if hits, err = s.Search(ctx, scope, query); err != nil || len(hits) != 0 {
		t.Fatal("suppressed evidence resurrected", hits, err)
	}
}

type remoteEmbedding struct{ calls int }

func (e *remoteEmbedding) Identity() string { return fake.Embedding{}.Identity() }
func (e *remoteEmbedding) Local() bool      { return false }
func (e *remoteEmbedding) Embed(ctx context.Context, text string) ([]float64, error) {
	e.calls++
	return fake.Embedding{}.Embed(ctx, text)
}
func TestEncryptedMemoryRetainsSemanticRecallAndRejectsRemoteEmbedding(t *testing.T) {
	s := openTest(t)
	key := memory.New()
	if e := key.Put(ctx, "key", bytes.Repeat([]byte{6}, 32)); e != nil {
		t.Fatal(e)
	}
	if e := s.ConfigureVault(mem.Vault{Secrets: key, Ref: "key"}); e != nil {
		t.Fatal(e)
	}
	if e := s.Curate(ctx, "private", scope, mem.SourceRef{Type: "conversation", Ref: "conversation:private"}, time.Unix(100, 0), []string{"I prefer chai without sugar"}, false, ""); e != nil {
		t.Fatal(e)
	}
	hits, e := s.Search(ctx, scope, recall.Query{Query: "tea"})
	if e != nil || len(hits) != 1 || hits[0].Content != "I prefer chai without sugar" {
		t.Fatal("private vector-only synonym failed", hits, e)
	}
	remote := &remoteEmbedding{}
	s.engine = remote
	if _, e = s.Search(ctx, scope, recall.Query{Query: "tea"}); e != recall.ErrUnavailable || remote.calls != 0 {
		t.Fatal("private data sent to remote provider", remote.calls, e)
	}
}

func TestPriorSensitiveMemorySealedBeforeProviderAdmission(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "memory.db")
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(schema + sqliteindex.Schema + `PRAGMA user_version=1;`); e != nil {
		t.Fatal(e)
	}
	const personal = "My health diagnosis: synthetic sensitive prior record"
	if _, e = db.Exec(`INSERT INTO memory_record(memory_id,user_id,scope_type,scope_id,tier,content,source_refs,created_at) VALUES('prior','alice','agent','collie','recall',?,'[{"source_type":"conversation","source_ref":"conversation:prior"}]',100000000)`, personal); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO recall_document(ref,user_id,agent_id,content,occurred_at,author_role,core,active) VALUES('memory:prior','alice','collie',?,100000000,'',0,1)`, personal); e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	s, e := Open(ctx, root)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	key := memory.New()
	if e = key.Put(ctx, "key", bytes.Repeat([]byte{4}, 32)); e != nil {
		t.Fatal(e)
	}
	if e = s.ConfigureVault(mem.Vault{Secrets: key, Ref: "key"}); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"memory.db", "memory.db-wal"} {
		raw, e := os.ReadFile(filepath.Join(root, name))
		if e != nil && !os.IsNotExist(e) {
			t.Fatal(e)
		}
		if bytes.Contains(raw, []byte(personal)) {
			t.Fatal("prior sensitive plaintext survived", name)
		}
	}
	var pending int
	if e = s.db.QueryRow(`SELECT cleanup_pending FROM memory_protection WHERE singleton=1`).Scan(&pending); e != nil || pending != 0 {
		t.Fatal(pending, e)
	}
	var source string
	if e = s.db.QueryRow(`SELECT source_refs FROM memory_record WHERE memory_id='prior'`).Scan(&source); e != nil || source != `[{"source_type":"conversation","source_ref":"conversation:prior"}]` {
		t.Fatal("migration lost provenance", source, e)
	}
}

func TestCategoryOptOutReplayDoesNotDeleteFreshExplicitEvidence(t *testing.T) {
	s := openTest(t)
	key := memory.New()
	if e := key.Put(ctx, "key", bytes.Repeat([]byte{5}, 32)); e != nil {
		t.Fatal(e)
	}
	if e := s.ConfigureVault(mem.Vault{Secrets: key, Ref: "key"}); e != nil {
		t.Fatal(e)
	}
	curate := func(id string, at int64, explicit bool) {
		t.Helper()
		if e := s.Curate(ctx, id, scope, mem.SourceRef{Type: "feedback", Ref: "feedback:" + id}, time.Unix(at, 0), []string{"My health report: I feel tired today"}, explicit, ""); e != nil {
			t.Fatal(e)
		}
	}
	curate("old", 100, false)
	if e := s.ForgetCategory(ctx, scope, "health", time.Unix(110, 0), "forget-health"); e != nil {
		t.Fatal(e)
	}
	curate("blocked", 120, false)
	var count int
	if e := s.db.QueryRow(`SELECT count(*) FROM memory_record WHERE deleted_at IS NULL`).Scan(&count); e != nil || count != 0 {
		t.Fatal("opt-out ignored", count, e)
	}
	curate("explicit", 130, true)
	if e := s.ForgetCategory(ctx, scope, "health", time.Unix(110, 0), "forget-health"); e != nil {
		t.Fatal(e)
	}
	if e := s.db.QueryRow(`SELECT count(*) FROM memory_record WHERE deleted_at IS NULL`).Scan(&count); e != nil || count != 1 {
		t.Fatal("replay deleted fresh explicit evidence", count, e)
	}
	if e := s.AllowCategory(ctx, scope, "health", time.Unix(140, 0), "allow-health"); e != nil {
		t.Fatal(e)
	}
	curate("allowed", 150, false)
	if e := s.ForgetCategory(ctx, scope, "health", time.Unix(160, 0), "forget-again"); e != nil {
		t.Fatal(e)
	}
	if e := s.AllowCategory(ctx, scope, "health", time.Unix(140, 0), "allow-health"); e != nil {
		t.Fatal(e)
	}
	curate("still-blocked", 170, false)
	if e := s.db.QueryRow(`SELECT count(*) FROM memory_record WHERE deleted_at IS NULL`).Scan(&count); e != nil || count != 0 {
		t.Fatal("old allow event undid later opt-out", count, e)
	}
}

type countedEmbedding struct {
	fake.Embedding
	calls int
}

func (e *countedEmbedding) Embed(ctx context.Context, text string) ([]float64, error) {
	e.calls++
	return e.Embedding.Embed(ctx, text)
}
func TestEncryptedRecallReusesVectorsAndInvalidatesScopeAndForget(t *testing.T) {
	s := openTest(t)
	engine := &countedEmbedding{}
	if err := s.Configure(ctx, engine); err != nil {
		t.Fatal(err)
	}
	secrets := memory.New()
	if err := secrets.Put(ctx, "key", bytes.Repeat([]byte{3}, 32)); err != nil {
		t.Fatal(err)
	}
	if err := s.ConfigureVault(mem.Vault{Secrets: secrets, Ref: "key"}); err != nil {
		t.Fatal(err)
	}
	add := func(id, text string) {
		t.Helper()
		if err := s.Curate(ctx, id, scope, mem.SourceRef{Type: "conversation", Ref: "conversation:" + id}, time.Unix(101, 0), []string{text}, true, ""); err != nil {
			t.Fatal(err)
		}
	}
	add("first", "I prefer green tea")
	add("second", "I travel by railway")
	q := recall.Query{Query: "tea"}
	if _, err := s.Search(ctx, scope, q); err != nil {
		t.Fatal(err)
	}
	if engine.calls != 3 {
		t.Fatal("expected two documents and query", engine.calls)
	}
	working := s.working
	if _, err := s.Search(ctx, scope, q); err != nil {
		t.Fatal(err)
	}
	if engine.calls != 4 || s.working != working {
		t.Fatal("unchanged encrypted corpus rebuilt", engine.calls)
	}
	add("third", "I enjoy restful sleep")
	if _, err := s.Search(ctx, scope, q); err != nil {
		t.Fatal(err)
	}
	if engine.calls != 6 {
		t.Fatal("unchanged vectors reembedded after append", engine.calls)
	}
	if err := s.Forget(ctx, scope, "first-0", time.Unix(110, 0)); err != nil {
		t.Fatal(err)
	}
	if s.working != nil || len(s.indexed) != 0 {
		t.Fatal("forgotten plaintext retained in working index")
	}
	hits, err := s.Search(ctx, scope, q)
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range hits {
		if hit.SourceRef == "memory:first-0" {
			t.Fatal("forgotten fact returned")
		}
	}
	other := recall.Scope{UserID: scope.UserID, AgentID: "other-agent"}
	if _, err := s.Search(ctx, other, q); err != nil {
		t.Fatal(err)
	}
	if s.working != nil {
		t.Fatal("plaintext index retained across scope switch")
	}
	// The alternate scope may have no encrypted facts and use the disk index.
	// The next encrypted search must still validate its exact authorized corpus.
	if _, err := s.Search(ctx, scope, q); err != nil {
		t.Fatal(err)
	}
	var vectors int
	if err := s.db.QueryRow(`SELECT count(*) FROM recall_document WHERE vector_json IS NOT NULL`).Scan(&vectors); err != nil || vectors != 0 {
		t.Fatal("encrypted vectors persisted", vectors, err)
	}
}

type blockingQueryEmbedding struct {
	fake.Embedding
	entered, release chan struct{}
}

func (e blockingQueryEmbedding) Embed(ctx context.Context, text string) ([]float64, error) {
	if text == "blocked query" {
		close(e.entered)
		select {
		case <-e.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return e.Embedding.Embed(ctx, text)
}
func TestEncryptedEmbeddingDoesNotBlockOtherRunsOrForget(t *testing.T) {
	s := openTest(t)
	engine := blockingQueryEmbedding{entered: make(chan struct{}), release: make(chan struct{})}
	if err := s.Configure(ctx, engine); err != nil {
		t.Fatal(err)
	}
	secrets := memory.New()
	secrets.Put(ctx, "key", bytes.Repeat([]byte{8}, 32))
	if err := s.ConfigureVault(mem.Vault{Secrets: secrets, Ref: "key"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Curate(ctx, "private", scope, mem.SourceRef{Type: "conversation", Ref: "source"}, time.Unix(101, 0), []string{"I prefer tea"}, true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Search(ctx, scope, recall.Query{Query: "tea"}); err != nil {
		t.Fatal(err)
	}
	work, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := s.Search(work, scope, recall.Query{Query: "blocked query"}); done <- err }()
	select {
	case <-engine.entered:
	case <-work.Done():
		t.Fatal("query did not start")
	}
	if _, err := s.Search(work, scope, recall.Query{Query: "railway"}); err != nil {
		t.Fatal("one query blocked another Run", err)
	}
	if err := s.Forget(work, scope, "private-0", time.Unix(110, 0)); err != nil {
		t.Fatal("embedding held privacy mutation", err)
	}
	close(engine.release)
	if err := <-done; err == nil {
		t.Fatal("closed pre-Forget working view returned private results")
	}
}
