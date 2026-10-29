package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/z-chenhao/collie/internal/memory"
	"github.com/z-chenhao/collie/internal/recall"
	"github.com/z-chenhao/collie/internal/recall/fake"
)

var ctx = context.Background()
var scope = recall.Scope{UserID: "alice", AgentID: "collie"}

func openTest(t *testing.T) *Store {
	t.Helper()
	s, e := Open(ctx, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	if e = s.Configure(ctx, fake.Embedding{}); e != nil {
		t.Fatal(e)
	}
	return s
}
func record(id, text string) memory.Record {
	return memory.Record{ID: id, UserID: "alice", ScopeType: "agent", ScopeID: "collie", Tier: "recall", Content: text, SourceRefs: []memory.SourceRef{{Type: "conversation", Ref: "conversation:original"}}, CreatedAt: time.Unix(100, 0).UTC()}
}
func put(t *testing.T, s *Store, r memory.Record) {
	t.Helper()
	if e := s.Apply(ctx, r); e != nil {
		t.Fatal(e)
	}
}

func TestHybridScopeHistoryAndForget(t *testing.T) {
	s := openTest(t)
	put(t, s, record("lexical", "tea schedule"))
	put(t, s, record("semantic", "chai preference"))
	put(t, s, record("other", "railway station"))
	user := record("user", "chai user preference")
	user.ScopeType = "user"
	user.ScopeID = "alice"
	put(t, s, user)
	foreign := record("bob", "tea foreign user")
	foreign.UserID = "bob"
	put(t, s, foreign)
	foreign = record("other-agent", "tea foreign agent")
	foreign.ScopeID = "other-agent"
	put(t, s, foreign)
	h, e := s.Search(ctx, scope, recall.Query{Query: "tea", Limit: 10})
	if e != nil {
		t.Fatal(e)
	}
	if len(h) != 3 || h[0].SourceRef != "memory:lexical" {
		t.Fatalf("FTS + vector fusion/scope: %+v", h)
	}
	for _, hit := range h {
		if hit.SourceRef == "memory:other" || hit.SourceRef == "memory:bob" || hit.SourceRef == "memory:other-agent" {
			t.Fatal(hit)
		}
	}
	if h[1].Content != "chai preference" {
		t.Fatal("vector-only synonym missing", h)
	}
	cut := time.Unix(101, 0)
	h, e = s.Search(ctx, scope, recall.Query{Query: "tea", TimeRange: &recall.TimeRange{Start: &cut}})
	if e != nil || len(h) != 0 {
		t.Fatal(h, e)
	}
	r := record("lexical-v2", "tea corrected preference")
	r.Supersedes = "lexical"
	put(t, s, r)
	put(t, s, r)
	r.Content = "drift"
	if e = s.Apply(ctx, r); e != recall.ErrConflict {
		t.Fatal(e)
	}
	if e = s.Forget(ctx, scope, "lexical-v2", time.Unix(102, 0)); e != nil {
		t.Fatal(e)
	}
	if e = s.Forget(ctx, scope, "lexical-v2", time.Unix(103, 0)); e != nil {
		t.Fatal(e)
	}
	h, e = s.Search(ctx, scope, recall.Query{Query: "tea"})
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range h {
		if v.SourceRef == "memory:lexical" || v.SourceRef == "memory:lexical-v2" {
			t.Fatal("forgotten lineage recalled", h)
		}
	}
	if e = s.Forget(ctx, scope, "bob", time.Now()); e != recall.ErrConflict {
		t.Fatal("cross-user forget", e)
	}
	var n int
	s.db.QueryRow(`SELECT count(*) FROM memory_record`).Scan(&n)
	if n != 7 {
		t.Fatal("audit lost", n)
	}
	if _, e = s.db.Exec(`UPDATE memory_record SET content='overwrite'`); e == nil {
		t.Fatal("mutable history")
	}
}

func TestRollbackConcurrentSupersedeAndIndependentSearch(t *testing.T) {
	s := openTest(t)
	put(t, s, record("base", "tea preference"))
	if _, e := s.db.Exec(`CREATE TRIGGER fault BEFORE INSERT ON recall_document BEGIN SELECT RAISE(ABORT,'fault'); END`); e != nil {
		t.Fatal(e)
	}
	putRecord := record("failed", "chai")
	putRecord.Supersedes = "base"
	if e := s.Apply(ctx, putRecord); e != recall.ErrUnavailable {
		t.Fatal(e)
	}
	var n int
	s.db.QueryRow(`SELECT count(*) FROM memory_record WHERE memory_id='failed'`).Scan(&n)
	if n != 0 {
		t.Fatal("partial mutation")
	}
	s.db.Exec(`DROP TRIGGER fault`)
	root := filepath.Dir(s.dbPath(t))
	other, e := Open(ctx, root)
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	other.Configure(ctx, fake.Embedding{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i, store := range []*Store{s, other} {
		wg.Add(1)
		go func(i int, store *Store) {
			defer wg.Done()
			r := record(fmt.Sprint("new-", i), "tea correction")
			r.Supersedes = "base"
			results <- store.Apply(ctx, r)
		}(i, store)
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if e == recall.ErrConflict {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h, e := s.Search(ctx, scope, recall.Query{Query: "tea"})
			if e != nil || len(h) != 1 {
				t.Error(h, e)
			}
		}()
	}
	wg.Wait()
	s.engine = fake.Embedding{Fail: true}
	if h, e := s.Search(ctx, scope, recall.Query{Query: "tea"}); e == nil || h != nil {
		t.Fatal("FTS fallback", h, e)
	}
}
func (s *Store) dbPath(t *testing.T) string {
	var n int
	var name, path string
	if e := s.db.QueryRow(`PRAGMA database_list`).Scan(&n, &name, &path); e != nil {
		t.Fatal(e)
	}
	return path
}

func TestMemoryMigrationEmptyPriorAndRollback(t *testing.T) {
	s := openTest(t)
	path := s.dbPath(t)
	s.Close()
	s, e := Open(ctx, filepath.Dir(path))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	var version int
	s.db.QueryRow(`PRAGMA user_version`).Scan(&version)
	if version != 2 {
		t.Fatal(version)
	}
	s.db.Exec(`PRAGMA user_version=0`)
	if e = s.migrate(ctx); e == nil {
		t.Fatal("conflicting schema accepted")
	}
	s.db.QueryRow(`PRAGMA user_version`).Scan(&version)
	if version != 0 {
		t.Fatal("failed migration committed", version)
	}
	s.db.Exec(`PRAGMA user_version=99`)
	if e = s.migrate(ctx); e == nil {
		t.Fatal("future schema accepted")
	}
}

func TestDomainRankingAndInclusiveTimeBoundary(t *testing.T) {
	s := openTest(t)
	for _, id := range []string{"a", "b", "c"} {
		r := record(id, "chai")
		if id == "c" {
			r.Tier = "core"
		}
		put(t, s, r)
	}
	// The lexical branch is empty: equal semantic ranks still use stable refs.
	h, e := s.Search(ctx, scope, recall.Query{Query: "tea"})
	if e != nil || len(h) != 3 || h[0].SourceRef != "memory:a" {
		t.Fatal(h, e)
	}
	// Both FTS and vector supply identical ranks. Exact time bounds retain all.
	at := time.Unix(100, 0)
	h, e = s.Search(ctx, scope, recall.Query{Query: "chai", TimeRange: &recall.TimeRange{Start: &at, End: &at}})
	if e != nil || len(h) != 3 {
		t.Fatal(h, e)
	}
	// Corrupt vector state may not silently drop to a lexical-only answer.
	s.db.Exec(`UPDATE recall_document SET vector_json='[1]' WHERE ref='memory:a'`)
	if h, e = s.Search(ctx, scope, recall.Query{Query: "chai"}); e == nil || h != nil {
		t.Fatal(h, e)
	}
}

type reversedEmbedding struct{}

func (reversedEmbedding) Identity() string { return "test-domain-ranking" }
func (reversedEmbedding) Embed(_ context.Context, text string) ([]float64, error) {
	if strings.Contains(text, "dominant") {
		return []float64{.5, .8660254037844386}, nil
	}
	return []float64{1, 0}, nil
}

func TestCoreTierBreaksFusedTieWithoutOverridingRelevance(t *testing.T) {
	for _, core := range []string{"a", "b", ""} {
		t.Run("core-"+core, func(t *testing.T) {
			s, e := Open(ctx, t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			if e = s.Configure(ctx, reversedEmbedding{}); e != nil {
				t.Fatal(e)
			}
			a := record("a", "tea tea tea tea dominant")
			b := record("b", "tea scarce")
			b.CreatedAt = b.CreatedAt.Add(time.Second)
			if core == "a" {
				a.Tier = "core"
			}
			if core == "b" {
				b.Tier = "core"
			}
			put(t, s, a)
			put(t, s, b)
			h, e := s.Search(ctx, scope, recall.Query{Query: "tea"})
			want := "memory:b"
			if core == "a" {
				want = "memory:a"
			}
			if e != nil || len(h) != 2 || h[0].SourceRef != want {
				t.Fatal(h, e, want)
			}
		})
	}
}
