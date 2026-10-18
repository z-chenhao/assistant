package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/recall"
	"github.com/z-chenhao/erii/internal/recall/fake"
)

func TestPriorConversationMigrationPreservesOriginalAndHybridRecall(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	db, e := sql.Open("sqlite", filepath.Join(root, "conversation.db"))
	if e != nil {
		t.Fatal(e)
	}
	// Exact M1 table columns; migration must not rewrite the original JSON or ID.
	_, e = db.Exec(`CREATE TABLE conversation_message(message_id TEXT PRIMARY KEY,conversation_id TEXT NOT NULL,sequence INTEGER NOT NULL,role TEXT NOT NULL,content_json TEXT NOT NULL,source TEXT NOT NULL,source_event_id TEXT,created_at INTEGER NOT NULL,UNIQUE(conversation_id,sequence)) STRICT;
INSERT INTO conversation_message VALUES('original','erii',1,'user','[{"type":"text","text":"chai preference"}]','cli','event',1000000);PRAGMA user_version=1;`)
	if e != nil {
		t.Fatal(e)
	}
	db.Close()
	s, e := Open(ctx, root)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Configure(ctx, fake.Embedding{}); e != nil {
		t.Fatal(e)
	}
	before, e := s.List(ctx, "erii")
	if e != nil {
		t.Fatal(e)
	}
	h, e := s.Search(ctx, recall.Scope{UserID: "local", AgentID: "erii"}, recall.Query{Query: "tea"})
	if e != nil || len(h) != 1 || h[0].SourceRef != "conversation:original" || h[0].AuthorRole != "user" || !h[0].OccurredAt.Equal(time.Unix(1, 0)) {
		t.Fatal(h, e)
	}
	after, _ := s.List(ctx, "erii")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("original rewritten")
	}
	var raw string
	s.db.QueryRow(`SELECT content_json FROM conversation_message`).Scan(&raw)
	if raw != `[{"type":"text","text":"chai preference"}]` {
		t.Fatal(raw)
	}
	if e = s.BindConversation(ctx, "different-channel", recall.Scope{UserID: "local", AgentID: "erii"}); e != nil {
		t.Fatal(e)
	}
	if e = s.BindConversation(ctx, "foreign", recall.Scope{UserID: "bob", AgentID: "erii"}); e != nil {
		t.Fatal(e)
	}
	for _, c := range []string{"different-channel", "foreign", "other-agent"} {
		if e = s.Append(ctx, []conversation.Message{{MessageID: c, ConversationID: c, Role: "assistant", Content: conversation.Text("tea evidence " + c), Source: "main", CreatedAt: time.Unix(2, 0)}}); e != nil {
			t.Fatal(e)
		}
	}
	h, e = s.Search(ctx, recall.Scope{UserID: "local", AgentID: "erii"}, recall.Query{Query: "tea"})
	if e != nil || len(h) != 2 || h[0].SourceRef != "conversation:different-channel" {
		t.Fatal(h, e)
	}
	if e = s.BindConversation(ctx, "different-channel", recall.Scope{UserID: "bob", AgentID: "erii"}); e != recall.ErrConflict {
		t.Fatal("rebound ownership", e)
	}
	for _, statement := range []string{`UPDATE conversation_scope SET user_id='bob'`, `DELETE FROM conversation_scope`, `INSERT OR REPLACE INTO conversation_scope VALUES('different-channel','bob','erii')`} {
		if _, e = s.db.Exec(statement); e == nil {
			t.Fatal("mutable persisted ownership", statement)
		}
	}
	if e = s.migrate(ctx); e != nil {
		t.Fatal("repeat migration", e)
	}
}
func TestConversationIndexRollbackAndFaultFailsClosed(t *testing.T) {
	ctx := context.Background()
	s, e := Open(ctx, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	s.Configure(ctx, fake.Embedding{})
	s.db.Exec(`CREATE TRIGGER index_fault BEFORE INSERT ON recall_document BEGIN SELECT RAISE(ABORT,'fault'); END`)
	m := conversation.Message{MessageID: "m", ConversationID: "erii", Role: "user", Content: conversation.Text("tea"), Source: "cli", CreatedAt: time.Now()}
	if e = s.Append(ctx, []conversation.Message{m}); e == nil {
		t.Fatal("partial append")
	}
	history, _ := s.List(ctx, "erii")
	if len(history) != 0 {
		t.Fatal(history)
	}
	s.db.Exec(`DROP TRIGGER index_fault`)
	if e = s.Append(ctx, []conversation.Message{m}); e != nil {
		t.Fatal(e)
	}
	s.db.Exec(`DROP TABLE recall_fts`)
	h, e := s.Search(ctx, recall.Scope{UserID: "local", AgentID: "erii"}, recall.Query{Query: "tea"})
	if e == nil || h != nil {
		t.Fatal("vector-only degraded success", h, e)
	}
}

type rejectEmptyEmbedding struct{ fake.Embedding }

func (e rejectEmptyEmbedding) Embed(ctx context.Context, text string) ([]float64, error) {
	if text == "" {
		return nil, recall.ErrUnavailable
	}
	return e.Embedding.Embed(ctx, text)
}
func TestMediaOnlyOriginalDoesNotBlockTextRecall(t *testing.T) {
	ctx := context.Background()
	s, e := Open(ctx, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	s.Configure(ctx, rejectEmptyEmbedding{})
	media := conversation.Message{MessageID: "media", ConversationID: "erii", Role: "user", Source: "cli", CreatedAt: time.Unix(1, 0), Content: []conversation.ContentPart{{Type: "image", ArtifactRef: "artifact:opaque"}}}
	text := media
	text.MessageID = "text"
	text.Content = conversation.Text("chai preference")
	if e = s.Append(ctx, []conversation.Message{media, text}); e != nil {
		t.Fatal(e)
	}
	h, e := s.Search(ctx, recall.Scope{UserID: "local", AgentID: "erii"}, recall.Query{Query: "tea"})
	if e != nil || len(h) != 1 || h[0].SourceRef != "conversation:text" {
		t.Fatal(h, e)
	}
	original, e := s.List(ctx, "erii")
	if e != nil || len(original) != 2 || original[0].Content[0].ArtifactRef != "artifact:opaque" {
		t.Fatal(original, e)
	}
}
