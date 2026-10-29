package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/z-chenhao/collie/internal/conversation"
)

func TestConversationOrderedIdempotentConcurrentAndRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, err := Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	other, err := Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	message := func(id string) conversation.Message {
		return conversation.Message{MessageID: id, ConversationID: "one", Role: "user", Content: conversation.Text(id), Source: "cli", SourceEventID: id, CreatedAt: time.UnixMicro(101).UTC()}
	}
	batch := []conversation.Message{message("a"), message("b")}
	if err = s.Append(ctx, batch); err != nil {
		t.Fatal(err)
	}
	if err = other.Append(ctx, batch); err != nil {
		t.Fatal(err)
	}
	drift := message("a")
	drift.Content = conversation.Text("changed")
	if err = other.Append(ctx, []conversation.Message{drift}); !errors.Is(err, conversation.ErrConflict) {
		t.Fatal("drift accepted", err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store := s
			if i%2 != 0 {
				store = other
			}
			errs <- store.Append(ctx, []conversation.Message{message(fmt.Sprint(i))})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.List(ctx, "one")
	if err != nil || len(first) != 18 {
		t.Fatal("message count", len(first), err)
	}
	for i, m := range first {
		if m.Sequence != int64(i+1) {
			t.Fatal("monotonic sequence violated")
		}
	}
	reopened, err := Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	after, err := reopened.List(ctx, "one")
	if err != nil || !reflect.DeepEqual(first, after) {
		t.Fatal("restart changed history", err)
	}
	if _, err = s.db.Exec(`UPDATE conversation_message SET content_json='[]'`); err == nil {
		t.Fatal("history mutable")
	}
	if _, err = s.db.Exec(`DELETE FROM conversation_message`); err == nil {
		t.Fatal("history deleted")
	}
	if err = s.Append(ctx, []conversation.Message{message("new"), drift}); !errors.Is(err, conversation.ErrConflict) {
		t.Fatal("batch drift", err)
	}
	final, _ := s.List(ctx, "one")
	if !reflect.DeepEqual(first, final) {
		t.Fatal("failed batch partially committed")
	}
}

func TestConversationFutureSchemaAndTraceContentRejected(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, err := Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	m := conversation.Message{MessageID: "tool", ConversationID: "one", Role: "assistant", Source: "main", CreatedAt: time.Now(), Content: []conversation.ContentPart{{Type: "tool_call", ToolCall: &conversation.ToolCall{ID: "one", Name: "name", Arguments: "{}"}}}}
	if err = s.Append(ctx, []conversation.Message{m}); !errors.Is(err, conversation.ErrInvalid) {
		t.Fatal("trace accepted in history", err)
	}
	if _, err = s.db.Exec(`PRAGMA user_version=99`); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if bad, err := Open(ctx, root); err == nil {
		bad.Close()
		t.Fatal("future schema opened")
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "conversation.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if db.QueryRow(`PRAGMA user_version`).Scan(&version) != nil || version != 99 {
		t.Fatal("future schema modified")
	}
}

func TestConversationPagesUseStableExclusiveCursorsAndScopedGet(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 205; i++ {
		m := conversation.Message{MessageID: fmt.Sprintf("m-%03d", i), ConversationID: "one", Role: "user", Content: conversation.Text("retained history"), Source: "cli", SourceEventID: fmt.Sprint(i), CreatedAt: time.UnixMicro(int64(i + 1))}
		if err := s.Append(ctx, []conversation.Message{m}); err != nil {
			t.Fatal(err)
		}
	}
	cursor := int64(0)
	seen := map[string]bool{}
	sizes := []int{}
	for {
		page, err := s.Page(ctx, "one", cursor)
		if err != nil {
			t.Fatal(err)
		}
		sizes = append(sizes, len(page.Messages))
		last := int64(0)
		for _, m := range page.Messages {
			if seen[m.MessageID] || m.Sequence <= last {
				t.Fatal("duplicate or unordered page", m)
			}
			seen[m.MessageID] = true
			last = m.Sequence
		}
		if page.Before == 0 {
			break
		}
		cursor = page.Before
	}
	if !reflect.DeepEqual(sizes, []int{100, 100, 5}) || len(seen) != 205 {
		t.Fatal("unbounded or incomplete history", sizes, len(seen))
	}
	if _, err := s.Get(ctx, "other", "m-000"); !errors.Is(err, conversation.ErrConflict) {
		t.Fatal("cross-conversation lookup", err)
	}
	if message, err := s.Get(ctx, "one", "m-000"); err != nil || message.Sequence != 1 {
		t.Fatal(message, err)
	}
	if _, err := s.Page(ctx, "one", -1); !errors.Is(err, conversation.ErrInvalid) {
		t.Fatal("bad cursor accepted")
	}
}
