package sqlite

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/skill"
	"github.com/z-chenhao/erii/internal/tool"
)

func TestExperienceRequiresIndependentVerifiedRunsAndCAS(t *testing.T) {
	ctx := context.Background()
	s, r, _, a, p := loopFixture(t)
	spec := tool.Spec{Name: "builtin__workspace__test", Version: 1, Permission: "read_only", ReplayPolicy: "safe", InputSchema: `{"type":"object","properties":{},"additionalProperties":false}`}
	registry, e := tool.NewRegistry([]tool.Definition{{Spec: spec, Execute: func(context.Context, string) tool.Result {
		return tool.Result{Text: `{"passed":true,"command":"go test -race ./...","truncated":false}`}
	}}})
	if e != nil {
		t.Fatal(e)
	}
	r.registry, a.Registry = registry, registry
	p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
		if req.Messages[len(req.Messages)-1].Role == "tool" {
			return llm.GenerationResult{Text: "Verified correction"}, nil
		}
		return calls(spec.Name, `{}`), nil
	}
	one := mustSubmit(t, a, "experience-1", "Verify the corrected method")
	mustDrain(t, a)
	two := mustSubmit(t, a, "experience-2", "Independently repeat the corrected method")
	mustDrain(t, a)
	c := skill.Config{Root: t.TempDir(), Scope: "agent"}
	q := skill.ReviewRequest{Operation: "propose", Name: "verified-tests", Description: "Verify a small repository correction.", Procedure: "1. Read AGENTS.md and reproduce the failure.\n2. Make the smallest correction.\n3. Run go test -race ./... and inspect the result.", Reason: "success", SourceRefs: []string{"run:" + one.RunID}}
	value, e := r.ReviewExperience(ctx, c, q)
	if e != nil {
		t.Fatal(e)
	}
	ref := value["candidate_ref"].(string)
	var inv1, inv2 string
	if e = s.db.QueryRow(`SELECT invocation_id FROM tool_invocation WHERE run_id=?`, one.RunID).Scan(&inv1); e != nil {
		t.Fatal(e)
	}
	if e = s.db.QueryRow(`SELECT invocation_id FROM tool_invocation WHERE run_id=?`, two.RunID).Scan(&inv2); e != nil {
		t.Fatal(e)
	}
	verify := func(version int64, inv string) map[string]any {
		v, e := r.ReviewExperience(ctx, c, skill.ReviewRequest{Operation: "verify", CandidateRef: ref, Version: version, VerificationRef: "invocation:" + inv})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	value = verify(1, inv1)
	if value["status"] != "candidate" {
		t.Fatal("single ordinary success promoted", value)
	}
	if _, e = r.ReviewExperience(ctx, c, skill.ReviewRequest{Operation: "promote", CandidateRef: ref, Version: 2}); e == nil {
		t.Fatal("unverified candidate promoted")
	}
	value = verify(2, inv2)
	if value["status"] != "verified" {
		t.Fatal(value)
	}
	if _, e = r.ReviewExperience(ctx, c, skill.ReviewRequest{Operation: "promote", CandidateRef: ref, Version: 2}); e == nil {
		t.Fatal("stale CAS")
	}
	value, e = r.ReviewExperience(ctx, c, skill.ReviewRequest{Operation: "promote", CandidateRef: ref, Version: 3})
	if e != nil || value["status"] != "promoted" {
		t.Fatal(value, e)
	}
	defs, e := skill.Definitions(c)
	if e != nil {
		t.Fatal(e)
	}
	var summaries []skill.Summary
	if json.Unmarshal([]byte(defs[0].Execute(ctx, `{}`).Text), &summaries) != nil || len(summaries) != 1 || summaries[0].Name != q.Name {
		t.Fatal(summaries)
	}
	for _, d := range defs {
		if d.Spec.Name == skill.ManageName {
			t.Fatal("ordinary Loop gained management")
		}
	}
	value, e = r.ReviewExperience(ctx, c, skill.ReviewRequest{Operation: "retire", CandidateRef: ref, Version: 4})
	if e != nil || value["status"] != "retired" {
		t.Fatal(value, e)
	}
	defs, e = skill.Definitions(c)
	if e != nil || defs[0].Execute(ctx, `{}`).Text != "[]" {
		t.Fatal(defs, e)
	}
}
