package memory

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSemanticProposalRequiresDurableAttributedEvidence(t *testing.T) {
	for _, text := range []string{"你给我调研一下，现在 grokbot 怎么用来赚钱，给我一个 roadmap", "我没看到 intelligent ui 的展示", "我刚才点按钮报错了", "帮我制作一个小游戏", "I am tired today, can you help me?"} {
		in := CurationInput{Observation: Observation{Author: "user", Text: text, At: time.Now()}}
		raw, _ := json.Marshal(Proposal{Facts: []Candidate{{Operation: "add", Facet: "user.goal", Category: "goals", Content: "invented lasting goal", Quote: text}}})
		if _, e := ParseProposal(string(raw), in); e == nil {
			t.Fatalf("transient request became memory: %s", text)
		}
	}
	in := CurationInput{Observation: Observation{Author: "user", Text: "我喜欢喝不加糖的绿茶", At: time.Now()}, Existing: []Fact{{Ref: "memory:one", Facet: "drinks.preference", Category: "preferences", Content: "用户偏好无糖绿茶", At: time.Now().Add(-time.Hour)}}}
	good := Candidate{Operation: "support", Target: "memory:one", Facet: "drinks.preference", Category: "preferences", Quote: in.Observation.Text}
	raw, _ := json.Marshal(Proposal{Facts: []Candidate{good}})
	if _, e := ParseProposal(string(raw), in); e != nil {
		t.Fatal(e)
	}
	for _, edit := range []func(*Candidate){func(c *Candidate) { c.Target = "memory:foreign" }, func(c *Candidate) { c.Quote = "我喜欢红茶" }, func(c *Candidate) { c.Operation = "execute" }, func(c *Candidate) { c.Category = "unknown" }} {
		bad := good
		edit(&bad)
		raw, _ := json.Marshal(Proposal{Facts: []Candidate{bad}})
		if _, e := ParseProposal(string(raw), in); e == nil {
			t.Fatal("unsafe proposal accepted", bad)
		}
	}
	in.Observation.Author = "worker"
	raw, _ = json.Marshal(Proposal{Facts: []Candidate{good}})
	if _, e := ParseProposal(string(raw), in); e == nil {
		t.Fatal("worker invented personal evidence")
	}
}

func TestProposalRequiresExplicitUncertaintyAndAcceptsPronounFreeFacts(t *testing.T) {
	in := CurationInput{Observation: Observation{Author: "user", Text: "常住上海", At: time.Now()}, Existing: []Fact{}}
	if !EligibleObservation(in.Observation) {
		t.Fatal("pronoun-free user fact was excluded")
	}
	if _, e := ParseProposal(`{"facts":[{"operation":"add","facet":"identity.home_city","category":"identity","content":"用户常住上海","quote":"常住上海"}]}`, in); e == nil {
		t.Fatal("missing confidence flag accepted")
	}
	in.Observation.Text = "我可能考虑独立开发"
	if _, e := ParseProposal(`{"facts":[{"operation":"add","facet":"goals.work","category":"goals","content":"用户从事独立开发","quote":"我可能考虑独立开发","tentative":false}]}`, in); e == nil {
		t.Fatal("tentative source became an established fact")
	}
}
