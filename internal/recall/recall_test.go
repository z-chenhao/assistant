package recall_test

import (
	"context"
	"encoding/json"
	"github.com/z-chenhao/erii/internal/recall"
	"strings"
	"testing"
	"time"
)

type provider struct {
	hits  []recall.Hit
	err   error
	start chan struct{}
	other chan struct{}
}

func (p provider) Search(ctx context.Context, _ recall.Scope, _ recall.Query) ([]recall.Hit, error) {
	if p.start != nil {
		close(p.start)
		select {
		case <-p.other:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return p.hits, p.err
}
func TestParallelSourcesBudgetAndNoScoreLeak(t *testing.T) {
	a, b := make(chan struct{}), make(chan struct{})
	s := recall.Service{Memory: provider{hits: []recall.Hit{{Content: "tea", SourceType: "memory", SourceRef: "m", OccurredAt: time.Unix(1, 0)}}, start: a, other: b}, Conversation: provider{hits: []recall.Hit{{Content: "chai", SourceType: "conversation", SourceRef: "c", AuthorRole: "user", OccurredAt: time.Unix(1, 0)}}, start: b, other: a}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	h, e := s.Search(ctx, recall.Scope{UserID: "u", AgentID: "a"}, recall.Query{Query: "tea"}, 1000)
	if e != nil || len(h) != 2 {
		t.Fatal(h, e)
	}
	raw, _ := json.Marshal(h)
	if strings.Contains(string(raw), "score") {
		t.Fatal(string(raw))
	}
	s.Memory = provider{err: recall.ErrUnavailable}
	s.Conversation = provider{hits: h}
	if h, e = s.Search(ctx, recall.Scope{UserID: "u", AgentID: "a"}, recall.Query{Query: "tea"}, 1000); e == nil || h != nil {
		t.Fatal("partial success", h, e)
	}
	s.Memory = provider{hits: []recall.Hit{{Content: strings.Repeat("x", 500)}}}
	s.Conversation = provider{hits: []recall.Hit{{Content: "short"}}}
	h, e = s.Search(ctx, recall.Scope{UserID: "u", AgentID: "a"}, recall.Query{Query: "tea"}, 150)
	if e != nil || len(h) != 1 || h[0].Content != "short" {
		t.Fatal(h, e)
	}
}
func TestSchemaRejectsAuthorityAndBadTime(t *testing.T) {
	for _, raw := range []string{`{"query":"tea","user_id":"bob"}`, `{"query":"tea","source":"memory"}`, `{"query":"tea","limit":0}`, `{"query":"tea","time_range":{"start":"yesterday"}}`, `{"query":"tea","time_range":{"start":"2026-02-01T00:00:00Z","end":"2026-01-01T00:00:00Z"}}`} {
		if _, e := recall.Parse(raw); e == nil {
			t.Fatal(raw)
		}
	}
}
