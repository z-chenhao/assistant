package sqlite

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/z-chenhao/assistant/internal/artifact"
	"github.com/z-chenhao/assistant/internal/evidence"
	"github.com/z-chenhao/assistant/internal/observer/github"
)

func TestGitHubLinkedEvidenceIsOwnerOnlyAndRechecksBinding(t *testing.T) {
	ctx := context.Background()
	s, r, a, _, cfg, taskRef := continuationFixture(t, "once")
	watchBatch(t, s, cfg, "")
	bindTask(t, s, cfg, taskRef)
	for _, key := range []string{"earlier-accepted", "later-accepted"} {
		watchBatch(t, s, cfg, key)
		mustDrain(t, a)
	}
	var sequence int64
	if err := s.db.QueryRow(`SELECT min(observation_sequence) FROM github_task_followup WHERE state='continued'`).Scan(&sequence); err != nil {
		t.Fatal(err)
	}
	githubRef := fmt.Sprintf("github:%d", sequence)
	q := evidence.Request{Ref: taskRef}
	page, err := r.ReadGitHubLinkedEvidence(ctx, q, &cfg)
	if err != nil || !hasEvidenceLink(page, githubRef) {
		t.Fatal("same-generation earlier evidence unavailable", err)
	}
	input := mustSubmit(t, a, "unrelated", "Read Task evidence")
	claim, err := r.Claim(ctx, input.RunID)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Release(ctx, claim)
	mainPage, err := r.readEvidence(ctx, q, claim)
	if err != nil || hasEvidenceLink(mainPage, githubRef) {
		t.Fatal("Main Task links exposed local GitHub evidence", err)
	}
	binding, err := s.GitHubTaskBinding(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	s.MainBoundary = func(point string) {
		if point == "after_evidence_read" {
			s.MainBoundary = nil
			if err := s.BindGitHubTask(ctx, cfg, github.BindingRequest{Version: binding.Version}, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	page, err = r.ReadGitHubLinkedEvidence(ctx, q, &cfg)
	if err != nil || hasEvidenceLink(page, githubRef) || page.Kind != "task" {
		t.Fatal("late unbind leaked a link or hid the original Task", err)
	}
	if page, err := r.ReadGitHubLinkedEvidence(ctx, evidence.Request{Ref: githubRef}, &cfg); !errors.Is(err, artifact.ErrNotFound) || page.Ref != "" {
		t.Fatal("direct access survived unbinding", err)
	}
}
