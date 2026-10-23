package task

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCanonicalSchemasAndStrictUnion(t *testing.T) {
	for _, input := range []string{`null`, `[]`, `{"operation":"create"}`, `{"operation":"create","task":""}`, `{"operation":"create","task":"ok","taskRef":"forbidden"}`, `{"operation":"create","task":"ok","context":"hidden"}`, `{"operation":"create","task":"ok","operation":"create"}`, `{"operation":"create","task":"ok","artifactRefs":null}`, `{"operation":"resume","taskRef":"x"}`, `{"operation":"append","taskRef":"x","instruction":null}`, `{"operation":"pause","taskRef":"x","instruction":"no"}`, `{"operation":"create","task":"ok"} {}`} {
		if _, err := ParseManage(input); err == nil {
			t.Fatal("accepted", input)
		}
	}
	for _, input := range []string{`{"operation":"create","task":"self-contained brief"}`, `{"operation":"append","taskRef":"opaque","instruction":"继续"}`, `{"operation":"pause","taskRef":"opaque"}`, `{"operation":"cancel","taskRef":"opaque"}`} {
		if _, err := ParseManage(input); err != nil {
			t.Fatal(input, err)
		}
	}
	for _, input := range []string{`{"limit":0}`, `{"limit":51}`, `{"limit":1.5}`, `{"status":["failed"]}`, `{"query":null}`, `{"limit":null}`, `{"scope":""}`, `{"unknown":true}`, `{"scope":"recent","scope":"all"}`} {
		if _, err := ParseSearch(input); err == nil {
			t.Fatal(input)
		}
	}
	a, _ := ParseSearch(`{}`)
	b, _ := ParseSearch(`{"query":"travel"}`)
	if a.Scope != "active" || b.Scope != "all" || a.Limit != 20 || b.Limit != 20 {
		t.Fatal(a, b)
	}
	if got := Title(strings.Repeat("旅", 90)); len([]rune(got)) != 80 {
		t.Fatal(got)
	}
	r := Registry()
	if len(r.Describe()) != 2 {
		t.Fatal(r.Describe())
	}
	for _, s := range r.Describe() {
		if (s.Name != ManageName && s.Name != SearchName) || !json.Valid([]byte(s.InputSchema)) {
			t.Fatal(s)
		}
	}
}
func TestSearchHybridAmbiguityAndStableOrder(t *testing.T) {
	cs := []Candidate{
		{Summary: Summary{TaskRef: "bridge", Status: "completed", UpdatedAt: "2026-10-01"}, Text: "wedding ceremony venue"},
		{Summary: Summary{TaskRef: "related", Status: "active", UpdatedAt: "2026-10-02"}, Text: "ceremony venue bookings"},
		{Summary: Summary{TaskRef: "unrelated", Status: "active", UpdatedAt: "2026-10-03"}, Text: "database migrations"},
		{Summary: Summary{TaskRef: "direct", Status: "active", UpdatedAt: "2026-10-01"}, Text: "wedding checklist"},
	}
	result := Rank(cs, "wedding", 50)
	if len(result) != 3 || result[0].TaskRef != "direct" || result[1].TaskRef != "related" || result[2].TaskRef != "bridge" {
		t.Fatal(result)
	}
	if got := Rank(cs, "unknown", 20); len(got) != 0 {
		t.Fatal(got)
	}
	if got := Rank(cs, "", 2); len(got) != 2 || got[0].TaskRef != "unrelated" {
		t.Fatal(got)
	}
}
