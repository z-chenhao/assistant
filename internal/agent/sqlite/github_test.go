package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/observer/github"
)

func githubConfig() github.Config {
	return github.Config{Repository: "owner/repo", PullNumber: 7, CredentialRef: "env:WATCH_TEST", PollSeconds: 5}
}
func githubBatch() github.Batch {
	return github.Batch{RepositoryID: 99, HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Events: []github.Observation{{Key: "comment:1:revision1", Kind: "review_comment", SourceID: 1, URL: "https://github.com/owner/repo/pull/7#discussion_r1", Body: "review feedback", UpdatedAt: time.Now().UTC()}}}
}

func TestGitHubDurableDedupAtomicFailureAndRepositoryIdentity(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "data")
	s, err := Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	cfg, batch := githubConfig(), githubBatch()
	poll := func(code string, b github.Batch) error {
		t.Helper()
		ticket, err := s.BeginGitHubPoll(ctx, cfg)
		if err != nil {
			return err
		}
		return s.FinishGitHubPoll(ctx, cfg, ticket, b, code)
	}
	if err = poll("", batch); err != nil {
		t.Fatal(err)
	}
	if err = poll("", batch); err != nil {
		t.Fatal(err)
	}
	if scalar(t, s, `SELECT count(*) FROM github_observation`) != 1 || scalar(t, s, `SELECT count(*) FROM event_outbox WHERE type='github.observation.received'`) != 1 || scalar(t, s, `SELECT count(*) FROM run`) != 0 {
		t.Fatal("duplicate event or work created")
	}
	status, err := s.GitHubStatus(ctx, cfg)
	if err != nil || status.State != "healthy" || status.LastSuccess == nil {
		t.Fatal(status, err)
	}
	priorSuccess := *status.LastSuccess
	batch.Events[0].Key = "comment:2:revision1"
	if err = poll("SOURCE_UNAVAILABLE", batch); err != nil {
		t.Fatal(err)
	}
	if err = poll("SOURCE_UNAVAILABLE", batch); err != nil {
		t.Fatal(err)
	}
	status, err = s.GitHubStatus(ctx, cfg)
	if err != nil || status.State != "interrupted" || !status.LastSuccess.Equal(priorSuccess) || status.EventCount != 1 || scalar(t, s, `SELECT count(*) FROM event_outbox WHERE type='github.watch.interrupted'`) != 1 {
		t.Fatal("partial scan advanced or health storm", status, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = poll("", batch); err != nil {
		t.Fatal(err)
	}
	if scalar(t, s, `SELECT count(*) FROM github_observation`) != 2 {
		t.Fatal("restart failed backfill")
	}
	batch.RepositoryID = 100
	if err = poll("", batch); err == nil {
		t.Fatal("repository identity changed silently")
	}
	status, err = s.GitHubStatus(ctx, cfg)
	if err != nil || status.FailureCode != "REPOSITORY_IDENTITY_CHANGED" || *status.RepositoryID != 99 {
		t.Fatal(status, err)
	}
}

func TestGitHubStopForgetAndConcurrentPollFence(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cfg, batch := githubConfig(), githubBatch()
	ticket, err := s.BeginGitHubPoll(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	newer, err := s.BeginGitHubPoll(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(s.FinishGitHubPoll(ctx, cfg, ticket, batch, ""), github.ErrStopped) {
		t.Fatal("stale scan committed")
	}
	if err = s.FinishGitHubPoll(ctx, cfg, newer, batch, ""); err != nil {
		t.Fatal(err)
	}
	ticket, err = s.BeginGitHubPoll(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetGitHubWatch(ctx, cfg, false); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(s.FinishGitHubPoll(ctx, cfg, ticket, batch, ""), github.ErrStopped) {
		t.Fatal("late result after stop committed")
	}
	if _, err = s.BeginGitHubPoll(ctx, cfg); !errors.Is(err, github.ErrStopped) {
		t.Fatal("poll revived stopped watch", err)
	}
	if err = s.SetGitHubWatch(ctx, cfg, true); err != nil {
		t.Fatal(err)
	}
	ticket, err = s.BeginGitHubPoll(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Transaction(ctx, func(tx *sql.Tx) error {
		var owner string
		if err := tx.QueryRow(`SELECT agent_id FROM agent_config`).Scan(&owner); err != nil {
			return err
		}
		return s.Runs(nil, nil).revokeMemory(ctx, tx, owner)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(s.FinishGitHubPoll(ctx, cfg, ticket, batch, ""), github.ErrStopped) {
		t.Fatal("Forget did not reject late result")
	}
	status, err := s.GitHubStatus(ctx, cfg)
	if err != nil || status.State != "privacy_stopped" || status.EventCount != 0 {
		t.Fatal(status, err)
	}
	if events, err := s.GitHubObservations(ctx, cfg, 0); err != nil || len(events) != 0 {
		t.Fatal("revoked observations exposed", events, err)
	}
	if _, err = s.BeginGitHubPoll(ctx, cfg); !errors.Is(err, github.ErrStopped) {
		t.Fatal("Forget intent automatically renewed", err)
	}
	if err = s.SetGitHubWatch(ctx, cfg, true); err != nil {
		t.Fatal(err)
	}
	ticket, err = s.BeginGitHubPoll(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	batch.Events[0].Key = "comment:2:revision1"
	if err = s.FinishGitHubPoll(ctx, cfg, ticket, batch, ""); err != nil {
		t.Fatal(err)
	}
	if events, err := s.GitHubObservations(ctx, cfg, 0); err != nil || len(events) != 1 || events[0].Key != "comment:2:revision1" {
		t.Fatal(events, err)
	}
}

func TestGitHubObservationAndWakeRollbackTogether(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cfg := githubConfig()
	ticket, err := s.BeginGitHubPoll(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`CREATE TRIGGER reject_github_wake BEFORE INSERT ON event_outbox WHEN NEW.type='github.observation.received' BEGIN SELECT RAISE(ABORT,'synthetic private SQL cause'); END;`); err != nil {
		t.Fatal(err)
	}
	err = s.FinishGitHubPoll(ctx, cfg, ticket, githubBatch(), "")
	var operation *agent.OperationError
	if !errors.As(err, &operation) || !errors.Is(err, agent.ErrStorage) || strings.Contains(err.Error(), "SQL") {
		t.Fatal("storage failure was not safe with retained cause", err)
	}
	if scalar(t, s, `SELECT count(*) FROM github_observation`) != 0 || scalar(t, s, `SELECT count(*) FROM event_outbox WHERE type='github.observation.received'`) != 0 {
		t.Fatal("observation escaped failed wake commit")
	}
	status, err := s.GitHubStatus(ctx, cfg)
	if err != nil || status.LastSuccess != nil || status.EventCount != 0 {
		t.Fatal("failed commit advanced success boundary", status, err)
	}
}
