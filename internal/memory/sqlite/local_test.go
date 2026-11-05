package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/assistant/internal/llm/memory"
	mem "github.com/z-chenhao/assistant/internal/memory"
	"github.com/z-chenhao/assistant/internal/recall"
)

func TestLocalSearchEncryptedMemoryRestartScopeAndForget(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	secrets := memory.New()
	if err := secrets.Put(ctx, "credential:key", []byte(strings.Repeat("k", 32))); err != nil {
		t.Fatal(err)
	}
	open := func() *Store {
		s, err := Open(ctx, root)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.ConfigureVault(mem.Vault{Secrets: secrets, Ref: "credential:key"}); err != nil {
			t.Fatal(err)
		}
		s.ConfigureLocal()
		return s
	}
	s := open()
	scope := recall.Scope{UserID: "local", AgentID: "one"}
	text := "我喜欢喝无糖绿茶"
	if err := s.Curate(ctx, "remember", scope, mem.SourceRef{Type: "feedback", Ref: "feedback:one"}, time.Unix(100, 0), []string{text}, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = open()
	defer s.Close()
	hits, err := s.Search(ctx, scope, recall.Query{Query: "无糖绿茶"})
	if err != nil || len(hits) != 1 || hits[0].Content != text {
		t.Fatal(hits, err)
	}
	hits, err = s.Search(ctx, recall.Scope{UserID: "other", AgentID: "one"}, recall.Query{Query: "无糖绿茶"})
	if err != nil || len(hits) != 0 {
		t.Fatal("scope leak", hits, err)
	}
	var bindings int
	if err = s.db.QueryRow(`SELECT count(*) FROM recall_configuration`).Scan(&bindings); err != nil || bindings != 0 {
		t.Fatal("local search bound a vector space", err)
	}
	for _, name := range []string{"memory.db", "memory.db-wal"} {
		raw, _ := os.ReadFile(filepath.Join(root, name))
		if strings.Contains(string(raw), text) {
			t.Fatal("plaintext persisted")
		}
	}
	if err = s.Forget(ctx, scope, "remember-0", time.Unix(101, 0)); err != nil {
		t.Fatal(err)
	}
	hits, err = s.Search(ctx, scope, recall.Query{Query: "无糖绿茶"})
	if err != nil || len(hits) != 0 {
		t.Fatal("forgotten result", hits, err)
	}
}
