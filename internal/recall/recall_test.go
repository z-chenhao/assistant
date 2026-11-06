package recall_test

import (
	"context"
	"encoding/json"
	"github.com/z-chenhao/assistant/internal/recall"
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

func TestHostExclusionsAndSourceContentDeduplication(t *testing.T) {
	scope := recall.Scope{UserID: "u", AgentID: "a"}
	s := recall.Service{Memory: provider{hits: []recall.Hit{
		{SourceRef: "memory:self", SourceType: "memory", Content: "normalized current input", Sources: []string{"conversation:current"}, Relevance: 1},
		{SourceRef: "memory:past", SourceType: "memory", Content: "normalized lasting fact", Sources: []string{"conversation:past"}, Relevance: .9},
	}}, Conversation: provider{hits: []recall.Hit{
		{SourceRef: "conversation:current", SourceType: "conversation", Content: "current input", Relevance: 1},
		{SourceRef: "conversation:past", SourceType: "conversation", Content: "original lasting fact", Relevance: .8},
		{SourceRef: "conversation:other", SourceType: "conversation", Content: " NORMALIZED   lasting FACT ", Relevance: .7},
		{SourceRef: "conversation:distinct", SourceType: "conversation", Content: "different relevant evidence", Relevance: .6},
	}}}
	hits, e := s.Search(context.Background(), scope, recall.Query{Query: "fact", ExcludeRefs: []string{"conversation:current"}}, 4096)
	if e != nil || len(hits) != 2 || hits[0].SourceRef != "memory:past" || hits[1].SourceRef != "conversation:distinct" {
		t.Fatal(hits, e)
	}
	raw, _ := json.Marshal(hits)
	if strings.Contains(string(raw), "Relevance") || strings.Contains(string(raw), "Sources") {
		t.Fatal("private ranking/provenance leaked", string(raw))
	}
	// Oversized derived text must not hide a source that fits the budget.
	s.Memory = provider{hits: []recall.Hit{{SourceRef: "memory:large", SourceType: "memory", Content: strings.Repeat("x", 1000), Sources: []string{"conversation:short"}, Relevance: 1}}}
	s.Conversation = provider{hits: []recall.Hit{{SourceRef: "conversation:short", SourceType: "conversation", Content: "short", Relevance: .9}}}
	hits, e = s.Search(context.Background(), scope, recall.Query{Query: "fact"}, 200)
	if e != nil || len(hits) != 1 || hits[0].SourceRef != "conversation:short" {
		t.Fatal(hits, e)
	}
}
