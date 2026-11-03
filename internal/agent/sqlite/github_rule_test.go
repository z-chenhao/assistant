package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/observer/github"
)

func TestGitHubRuleModesOnSameCandidate(t *testing.T) {
	for _, mode := range []string{"off", "observe", "automatic"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			s, _, a, p, cfg, ref := continuationFixture(t, "once")
			watchBatch(t, s, cfg, "")
			if mode != "off" {
				q := github.BindingRequest{TaskRef: ref, TaskEpoch: 1, Mode: mode, EventKind: "review_comment"}
				if err := s.BindGitHubTask(ctx, cfg, q, true); err != nil {
					t.Fatal(err)
				}
			}
			binding, err := s.GitHubTaskBinding(ctx, cfg)
			if err != nil || binding.Mode != mode || binding.Scope != cfg.Scope() {
				t.Fatal(binding, err)
			}
			if mode != "off" && (binding.TaskRef != ref || binding.TaskGoal == "" || binding.EventKind != "review_comment") {
				t.Fatal("missing original work scope", binding)
			}
			before := len(p.requests)
			runCount := scalar(t, s, `SELECT count(*) FROM run`)
			for range 2 {
				watchBatch(t, s, cfg, "same-review")
				mustDrain(t, a)
			}
			decisions, err := s.GitHubRuleDecisions(ctx, cfg, 0)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "off" && len(decisions) != 0 {
				t.Fatal("closed rule recorded a decision", decisions)
			}
			if mode != "off" && (len(decisions) != 1 || !decisions[0].Matched || decisions[0].Reason != "event_kind_matched" || decisions[0].TaskRef != ref || decisions[0].Mode != mode || decisions[0].Generation != binding.Generation) {
				t.Fatal("missing or duplicate recorded decision", decisions)
			}
			if mode == "automatic" {
				if scalar(t, s, `SELECT count(*) FROM github_task_followup WHERE state='continued'`) != 1 ||
					scalar(t, s, `SELECT count(*) FROM task_instruction`) != 1 ||
					scalar(t, s, `SELECT count(*) FROM task`) != 1 {
					t.Fatal("new matching observation did not continue the same Task exactly once")
				}
			} else if len(p.requests) != before || scalar(t, s, `SELECT count(*) FROM run`) != runCount || scalar(t, s, `SELECT count(*) FROM task_instruction`) != 0 {
				t.Fatal("off/observe mode executed work or used a model")
			}
		})
	}
}

func TestGitHubRuleRevisionExcludesBacklogAndLatePollAcrossRestart(t *testing.T) {
	ctx := context.Background()
	s, r, a, p, cfg, ref := continuationFixture(t, "once")
	watchBatch(t, s, cfg, "")
	q := github.BindingRequest{TaskRef: ref, TaskEpoch: 1, Mode: "observe", EventKind: "review_comment"}
	if err := s.BindGitHubTask(ctx, cfg, q, true); err != nil {
		t.Fatal(err)
	}
	watchBatch(t, s, cfg, "observed-only")
	original, err := s.GitHubTaskBinding(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	late, err := s.BeginGitHubPoll(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	q.Mode = "automatic"
	if err = s.BindGitHubTask(ctx, cfg, q, true); !errors.Is(err, agent.ErrConflict) {
		t.Fatal("stale rule revision was accepted", err)
	}
	q.Version = original.Version
	if err = s.BindGitHubTask(ctx, cfg, q, true); err != nil {
		t.Fatal(err)
	}
	batch := githubBatch()
	batch.Events[0].Key = "late-old-generation"
	if err = s.FinishGitHubPoll(ctx, cfg, late, batch, ""); err != nil {
		t.Fatal(err)
	}
	watchBatch(t, s, cfg, "observed-only")
	watchBatch(t, s, cfg, "late-old-generation")
	mustDrain(t, a)
	if scalar(t, s, `SELECT count(*) FROM github_observation`) != 2 || scalar(t, s, `SELECT count(*) FROM github_rule_decision`) != 1 ||
		scalar(t, s, `SELECT count(*) FROM github_task_followup`) != 0 {
		t.Fatal("old generation was re-evaluated or the late original was lost")
	}
	batch = githubBatch()
	batch.Events[0].Key = "new-matching"
	for i := range 51 {
		batch.Events = append(batch.Events, github.Observation{Kind: "review", Key: fmt.Sprintf("new-nonmatch-%d", i), SourceID: int64(i + 2)})
	}
	ticket, err := s.BeginGitHubPoll(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.FinishGitHubPoll(ctx, cfg, ticket, batch, ""); err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(s.path)
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.ConfigureGitHub(ctx, &cfg); err != nil {
		t.Fatal(err)
	}
	reopened := s.Runs(r.history, r.registry)
	a.Store = reopened
	p.store = s
	mustDrain(t, a)
	watchBatch(t, s, cfg, "new-matching")
	mustDrain(t, a)
	if scalar(t, s, `SELECT count(*) FROM github_task_followup WHERE state='continued'`) != 1 || scalar(t, s, `SELECT count(*) FROM task_instruction`) != 1 {
		t.Fatal("restart admitted an unmatched/old event or lost the matching one")
	}
	first, err := s.GitHubRuleDecisions(ctx, cfg, 0)
	if err != nil || len(first) != 50 {
		t.Fatal("decision page did not honor its limit", len(first), err)
	}
	for _, d := range first {
		if d.Matched || d.Reason != "event_kind_mismatch" {
			t.Fatal("mismatch was not explained", d)
		}
	}
	last, err := s.GitHubRuleDecisions(ctx, cfg, first[len(first)-1].Sequence)
	if err != nil || len(last) != 3 || !last[1].Matched || last[1].Mode != "automatic" ||
		last[2].EventKey != "observed-only" || last[2].Generation != original.Generation || last[2].Mode != "observe" {
		t.Fatal("paging changed historical decisions", last, err)
	}

	mustSubmit(t, a, "other-owner-task", "Create a second owner Task")
	mustDrain(t, a)
	var other string
	if err = s.db.QueryRow(`SELECT task_ref FROM task WHERE task_ref!=?`, ref).Scan(&other); err != nil {
		t.Fatal(err)
	}
	current, err := s.GitHubTaskBinding(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	q = github.BindingRequest{TaskRef: other, TaskEpoch: 1, Version: current.Version, Mode: "observe", EventKind: "review"}
	if err = s.BindGitHubTask(ctx, cfg, q, true); err != nil {
		t.Fatal(err)
	}
	watchBatch(t, s, cfg, "new-task-rule")
	decisions, err := s.GitHubRuleDecisions(ctx, cfg, 0)
	if err != nil || len(decisions) != 50 || decisions[0].TaskRef != other || decisions[1].TaskRef != ref {
		t.Fatal("rebind rewrote historical work scope", err)
	}
	otherScope := cfg
	otherScope.PullNumber++
	if decisions, err = s.GitHubRuleDecisions(ctx, otherScope, 0); err != nil || len(decisions) != 0 {
		t.Fatal("decision query leaked another PR", decisions, err)
	}
	owner := idAgent(t, s)
	if err = s.Transaction(ctx, func(tx *sql.Tx) error { return reopened.revokeMemory(ctx, tx, owner) }); err != nil {
		t.Fatal(err)
	}
	if decisions, err = s.GitHubRuleDecisions(ctx, cfg, 0); err != nil || len(decisions) != 0 {
		t.Fatal("decision query retained forgotten history", decisions, err)
	}
}

func TestGitHubRuleMigrationPreservesLegacyBindingAndPendingObservation(t *testing.T) {
	ctx := context.Background()
	s, r, a, p, cfg, ref := continuationFixture(t, "once")
	watchBatch(t, s, cfg, "")
	bindTask(t, s, cfg, ref)
	watchBatch(t, s, cfg, "already-observed")
	if _, err := s.db.Exec(`DROP TABLE github_rule_decision;
 ALTER TABLE github_task_binding DROP COLUMN rule_event_kind;
 ALTER TABLE github_task_binding DROP COLUMN rule_mode;
 ALTER TABLE github_observation DROP COLUMN binding_generation;
 PRAGMA user_version=37`); err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(s.path)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if err = upgraded.ConfigureGitHub(ctx, &cfg); err != nil {
		t.Fatal(err)
	}
	binding, err := upgraded.GitHubTaskBinding(ctx, cfg)
	if err != nil || binding.Mode != "automatic" || binding.EventKind != "" || binding.TaskRef != ref || binding.Generation != 1 || binding.Version != 1 {
		t.Fatal("upgrade changed the existing owner's authorization", binding, err)
	}
	a.Store = upgraded.Runs(r.history, r.registry)
	p.store = upgraded
	mustDrain(t, a)
	if scalar(t, upgraded, `PRAGMA user_version`) != 38 ||
		scalar(t, upgraded, `SELECT count(*) FROM github_task_followup WHERE state='continued'`) != 1 ||
		scalar(t, upgraded, `SELECT count(*) FROM github_rule_decision`) != 0 {
		t.Fatal("migration lost existing automatic behavior or inferred a new rule")
	}
}

func TestGitHubRuleLatePollCannotGainLegacyAuthority(t *testing.T) {
	ctx := context.Background()
	s, _, a, _, cfg, ref := continuationFixture(t, "once")
	watchBatch(t, s, cfg, "")
	if err := s.BindGitHubTask(ctx, cfg, github.BindingRequest{TaskRef: ref, TaskEpoch: 1, Mode: "observe", EventKind: "review_comment"}, true); err != nil {
		t.Fatal(err)
	}
	current, err := s.GitHubTaskBinding(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := s.BeginGitHubPoll(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.BindGitHubTask(ctx, cfg, github.BindingRequest{TaskRef: ref, TaskEpoch: 1, Version: current.Version}, true); err != nil {
		t.Fatal(err)
	}
	batch := githubBatch()
	batch.Events[0].Key = "old-observe-ticket"
	if err = s.FinishGitHubPoll(ctx, cfg, ticket, batch, ""); err != nil {
		t.Fatal(err)
	}
	watchBatch(t, s, cfg, "old-observe-ticket")
	mustDrain(t, a)
	if scalar(t, s, `SELECT count(*) FROM github_observation`) != 1 || scalar(t, s, `SELECT count(*) FROM github_task_followup`) != 0 {
		t.Fatal("legacy bind promoted a late observation from another generation")
	}
	watchBatch(t, s, cfg, "new-legacy-ticket")
	mustDrain(t, a)
	if scalar(t, s, `SELECT count(*) FROM github_task_followup WHERE state='continued'`) != 1 {
		t.Fatal("legacy bind did not continue a newly received observation")
	}
}

func TestGitHubRuleCloseKeepsAcceptedRoot(t *testing.T) {
	ctx := context.Background()
	s, _, a, _, cfg, ref := continuationFixture(t, "once")
	watchBatch(t, s, cfg, "")
	if err := s.BindGitHubTask(ctx, cfg, github.BindingRequest{TaskRef: ref, TaskEpoch: 1, Mode: "automatic", EventKind: "review_comment"}, true); err != nil {
		t.Fatal(err)
	}
	watchBatch(t, s, cfg, "accepted-root")
	var closed atomic.Bool
	s.MainBoundary = func(point string) {
		if point != "before_worker_generation" || !closed.CompareAndSwap(false, true) {
			return
		}
		binding, err := s.GitHubTaskBinding(ctx, cfg)
		if err != nil {
			t.Error(err)
			return
		}
		if err = s.BindGitHubTask(ctx, cfg, github.BindingRequest{Version: binding.Version}, false); err != nil {
			t.Error(err)
		}
	}
	mustDrain(t, a)
	binding, err := s.GitHubTaskBinding(ctx, cfg)
	if err != nil || binding.Mode != "off" || !closed.Load() ||
		scalar(t, s, `SELECT count(*) FROM task_instruction`) != 1 ||
		scalar(t, s, `SELECT count(*) FROM run WHERE worker_position='root' AND status='completed'`) != 2 {
		t.Fatal("closing the rule interrupted an already accepted Root", binding, err)
	}
}
