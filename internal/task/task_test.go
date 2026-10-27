package task

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/tool"
)

func TestCompleteContract(t *testing.T) {
	raw := `{"operation":"complete","taskRef":"task:opaque"}`
	if _, err := ParseManage(raw); err != nil || !tool.Validate(ManageSchema, raw) {
		t.Fatalf("complete rejected: %v", err)
	}
	for _, raw := range []string{
		`{"operation":"complete"}`,
		`{"operation":"complete","taskRef":null}`,
		`{"operation":"complete","taskRef":""}`,
		`{"operation":"complete","taskRef":"x","instruction":"finish"}`,
		`{"operation":"complete","taskRef":"x","completionPolicy":"once"}`,
		`{"operation":"complete","taskRef":"x","artifactRefs":[]}`,
		`{"operation":"complete","taskRef":"x","taskRef":"y"}`,
	} {
		if _, err := ParseManage(raw); err == nil {
			t.Fatal("invalid complete accepted", raw)
		}
	}
}

func TestCreateCompletionPolicyContract(t *testing.T) {
	for _, policy := range []string{"", "once", "ongoing"} {
		raw := `{"operation":"create","task":"Keep the wedding checklist current"}`
		if policy != "" {
			raw = strings.TrimSuffix(raw, "}") + `,"completionPolicy":"` + policy + `"}`
		}
		r, err := ParseManage(raw)
		if err != nil || !tool.Validate(ManageSchema, raw) {
			t.Fatalf("policy %q rejected: %v", policy, err)
		}
		// once and omission share the pre-extension canonical request, so saved
		// invocation/source hashes remain valid across an upgrade.
		canonical, _ := json.Marshal(r)
		if policy != "ongoing" && string(canonical) != `{"operation":"create","task":"Keep the wedding checklist current"}` {
			t.Fatalf("one-shot canonical request changed: %s", canonical)
		}
		if policy == "ongoing" && !strings.Contains(string(canonical), `"completionPolicy":"ongoing"`) {
			t.Fatalf("ongoing policy lost: %s", canonical)
		}
	}
	for _, raw := range []string{
		`{"operation":"create","task":"x","completionPolicy":""}`,
		`{"operation":"create","task":"x","completionPolicy":"forever"}`,
		`{"operation":"create","task":"x","completionPolicy":null}`,
		`{"operation":"create","task":"x","completionPolicy":1}`,
		`{"operation":"create","task":"x","completionPolicy":"once","completionPolicy":"ongoing"}`,
		`{"operation":"append","taskRef":"x","instruction":"continue","completionPolicy":"ongoing"}`,
		`{"operation":"pause","taskRef":"x","completionPolicy":"once"}`,
		`{"operation":"cancel","taskRef":"x","completionPolicy":"ongoing"}`,
	} {
		if _, err := ParseManage(raw); err == nil {
			t.Fatal("invalid policy accepted", raw)
		}
	}
}

func TestPriorManageSchemaFailsClosed(t *testing.T) {
	// Simulate a frozen pre-extension v1 capability without rewriting it.
	defs := Definitions()
	var prior tool.Spec
	for i := range defs {
		if defs[i].Spec.Name != ManageName {
			continue
		}
		var schema map[string]any
		if err := json.Unmarshal([]byte(defs[i].Spec.InputSchema), &schema); err != nil {
			t.Fatal(err)
		}
		schema["oneOf"] = schema["oneOf"].([]any)[:4]
		create := schema["oneOf"].([]any)[0].(map[string]any)
		delete(create["properties"].(map[string]any), "completionPolicy")
		raw, _ := json.Marshal(schema)
		defs[i].Spec.InputSchema, defs[i].Spec.Description = string(raw), ""
		prior = defs[i].Spec
	}
	old, err := tool.NewRegistry(defs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = old.Select([]tool.Spec{prior}); err != nil {
		t.Fatal(err)
	}
	if _, err = Registry().Select([]tool.Spec{prior}); err == nil {
		t.Fatal("prior frozen schema silently replaced")
	}
	if tool.Validate(prior.InputSchema, `{"operation":"complete","taskRef":"x"}`) {
		t.Fatal("old schema gained complete")
	}
	if tool.Validate(prior.InputSchema, `{"operation":"create","task":"x","completionPolicy":"ongoing"}`) {
		t.Fatal("old schema gained new argument")
	}
}

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
