package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/z-chenhao/assistant/internal/observer/github"
)

const removeGitHubFailedCheckFixtureSchema = `DROP TABLE IF EXISTS ui_widget;
 ALTER TABLE github_rule_decision DROP COLUMN actual_check_conclusion;
 ALTER TABLE github_rule_decision DROP COLUMN expected_check_conclusion;
 ALTER TABLE github_task_binding DROP COLUMN rule_check_conclusion;
`

func checkBatch(key, state string) github.Batch {
	batch := githubBatch()
	batch.Events[0].Key = key
	batch.Events[0].Kind = "check_run"
	batch.Events[0].State = state
	return batch
}

func TestGitHubFailedCheckRuleKeepsDecisionsAndAuthorityPerGeneration(t *testing.T) {
	ctx := context.Background()
	s, _, a, p, cfg, ref := continuationFixture(t, "once")
	watchBatch(t, s, cfg, "")
	q := github.BindingRequest{TaskRef: ref, TaskEpoch: 1, Mode: "observe", EventKind: "check_run", CheckConclusion: "failure"}
	bind := func() {
		t.Helper()
		current, err := s.GitHubTaskBinding(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		q.Version = current.Version
		if err = s.BindGitHubTask(ctx, cfg, q, true); err != nil {
			t.Fatal(err)
		}
	}
	begin := func() github.Ticket {
		t.Helper()
		ticket, err := s.BeginGitHubPoll(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		return ticket
	}
	finish := func(ticket github.Ticket, key, state string) {
		t.Helper()
		if err := s.FinishGitHubPoll(ctx, cfg, ticket, checkBatch(key, state), ""); err != nil {
			t.Fatal(err)
		}
	}
	decisions := func() map[string]github.RuleDecision {
		t.Helper()
		rows, err := s.GitHubRuleDecisions(ctx, cfg, 0)
		if err != nil {
			t.Fatal(err)
		}
		byKey := make(map[string]github.RuleDecision, len(rows))
		for _, row := range rows {
			byKey[row.EventKey] = row
		}
		return byKey
	}
	bind()
	before := len(p.requests)
	finish(begin(), "observed-failure", "failure")
	finish(begin(), "observed-success", "success")
	finish(begin(), "observed-malformed", " FAILURE ")
	mustDrain(t, a)
	observed := decisions()
	if len(p.requests) != before || len(observed) != 3 || !observed["observed-failure"].Matched ||
		observed["observed-failure"].Reason != "check_conclusion_matched" ||
		observed["observed-success"].Matched || observed["observed-success"].ActualCheckConclusion != "success" ||
		observed["observed-malformed"].Matched || observed["observed-malformed"].ActualCheckConclusion != "unknown" {
		t.Fatal("observe mode executed work or recorded an incorrect check decision", observed)
	}
	late := begin()
	q.Mode = "automatic"
	bind()
	finish(late, "old-generation-failure", "failure")
	finish(begin(), "old-generation-failure", "failure")
	finish(begin(), "observed-failure", "failure")
	finish(begin(), "automatic-success", "success")
	finish(begin(), "automatic-malformed", "Failure")
	mustDrain(t, a)
	if len(p.requests) != before || scalar(t, s, `SELECT count(*) FROM github_task_followup`) != 0 {
		t.Fatal("success, malformed conclusion or an old generation authorized work")
	}
	for range 2 {
		finish(begin(), "automatic-failure", "failure")
		mustDrain(t, a)
	}
	if scalar(t, s, `SELECT count(*) FROM github_task_followup WHERE state='continued'`) != 1 ||
		scalar(t, s, `SELECT count(*) FROM task_instruction`) != 1 || scalar(t, s, `SELECT count(*) FROM task`) != 1 {
		t.Fatal("a failed check did not continue the same Task exactly once")
	}
	q.TaskEpoch = 2
	q.CheckConclusion = ""
	bind()
	finish(begin(), "type-only-success", "success")
	mustDrain(t, a)
	current := decisions()
	if len(current) != 7 || current["observed-failure"] != observed["observed-failure"] ||
		current["observed-success"] != observed["observed-success"] ||
		current["automatic-success"].ExpectedCheckConclusion != "failure" || current["automatic-success"].Reason != "check_conclusion_mismatch" ||
		!current["type-only-success"].Matched || current["type-only-success"].ExpectedCheckConclusion != "" ||
		current["type-only-success"].ActualCheckConclusion != "" || current["type-only-success"].Reason != "event_kind_matched" ||
		scalar(t, s, `SELECT count(*) FROM task_instruction`) != 2 {
		t.Fatal("editing the rule changed history or narrowed the type-only selector", current)
	}
}

func TestGitHubFailedCheckMigrationRetainsQueuedTypeOnlyDecision(t *testing.T) {
	ctx := context.Background()
	s, r, a, p, cfg, ref := continuationFixture(t, "once")
	watchBatch(t, s, cfg, "")
	if err := s.BindGitHubTask(ctx, cfg, github.BindingRequest{TaskRef: ref, TaskEpoch: 1, Mode: "automatic", EventKind: "check_run"}, true); err != nil {
		t.Fatal(err)
	}
	ticket, err := s.BeginGitHubPoll(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.FinishGitHubPoll(ctx, cfg, ticket, checkBatch("queued-v40", "success"), ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(removeGitHubFailedCheckFixtureSchema + `PRAGMA user_version=40`); err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(s.path)
	if err = s.Close(); err != nil {
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
	rows, err := upgraded.GitHubRuleDecisions(ctx, cfg, 0)
	if err != nil || len(rows) != 1 || !rows[0].Matched || rows[0].Reason != "event_kind_matched" ||
		rows[0].ExpectedCheckConclusion != "" || rows[0].ActualCheckConclusion != "" {
		t.Fatal("migration changed the old decision", rows, err)
	}
	a.Store = upgraded.Runs(r.history, r.registry)
	p.store = upgraded
	mustDrain(t, a)
	if scalar(t, upgraded, `PRAGMA user_version`) != SchemaVersion() ||
		scalar(t, upgraded, `SELECT count(*) FROM github_task_followup WHERE state='continued'`) != 1 {
		t.Fatal("old queued type-only match did not continue after upgrade")
	}
}
