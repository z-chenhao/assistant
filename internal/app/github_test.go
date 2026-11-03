package app

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/z-chenhao/assistant/internal/config"
	secretmemory "github.com/z-chenhao/assistant/internal/llm/memory"
	"github.com/z-chenhao/assistant/internal/observer/github"
	"github.com/z-chenhao/assistant/internal/skill"
)

func TestStartupReconcilesRemovedOrChangedGitHubScopeBeforeActor(t *testing.T) {
	for _, next := range []string{"absent", "different"} {
		t.Run(next, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			cfg, err := config.Load("", func(key string) (string, bool) { return root, key == "STEWARD_DATA_ROOT" })
			if err != nil {
				t.Fatal(err)
			}
			original := github.Config{Repository: "owner/repo", PullNumber: 7, CredentialRef: "env:GITHUB_STARTUP_SYNTHETIC"}
			cfg.GitHub = &original
			a, err := open(ctx, cfg, "", secretmemory.New(), true)
			if err != nil {
				t.Fatal(err)
			}
			// An exact persisted state fixture isolates startup composition. Domain
			// acceptance/late model/tool fencing is exercised through Actor in sqlite.
			d, err := a.store.Diagnostics(ctx)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UnixMicro()
			err = a.store.Transaction(ctx, func(tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, `INSERT INTO task(task_id,task_ref,agent_id,conversation_id,title,anchor_task,anchor_artifact_refs,status,task_epoch,created_at,updated_at,version,completion_policy) VALUES('startup-task','task:startup',?,?,'task','exact PR','[]','active',1,?,?,1,'ongoing')`, d.AgentID, d.AgentID, now, now)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			ticket, err := a.store.BeginGitHubPoll(ctx, original)
			if err != nil {
				t.Fatal(err)
			}
			if err = a.store.FinishGitHubPoll(ctx, original, ticket, github.Batch{RepositoryID: 99, HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, ""); err != nil {
				t.Fatal(err)
			}
			if _, err = a.BindGitHubTask(ctx, github.BindingRequest{TaskRef: "task:startup", TaskEpoch: 1}, true); err != nil {
				t.Fatal(err)
			}
			ticket, err = a.store.BeginGitHubPoll(ctx, original)
			if err != nil {
				t.Fatal(err)
			}
			if err = a.store.FinishGitHubPoll(ctx, original, ticket, github.Batch{RepositoryID: 99, HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Events: []github.Observation{{Key: "pending-before-config-change", Kind: "review", SourceID: 8, Body: "original scoped review"}}}, ""); err != nil {
				t.Fatal(err)
			}
			if err = a.Close(); err != nil {
				t.Fatal(err)
			}
			cfg.GitHub = nil
			if next == "different" {
				other := original
				other.PullNumber++
				cfg.GitHub = &other
			}
			cfg.Skills = &skill.Config{Root: filepath.Join(root, "missing-skills"), Scope: "agent"}
			a, err = OpenControl(ctx, cfg, "", "")
			if err != nil {
				t.Fatal(err)
			}
			// Real control composition and read-only projections cannot change a
			// separately executing runtime's scope, even when this config differs.
			if _, err = a.GitHubStatus(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err = a.GitHubTaskBinding(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err = a.Models().ListProfiles(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err = a.store.Inspector().ComputerActivity(ctx); err != nil {
				t.Fatal(err)
			}
			if cfg.GitHub != nil {
				if _, err = a.BindGitHubTask(ctx, github.BindingRequest{TaskRef: "task:startup", TaskEpoch: 1, Version: 1}, true); err == nil {
					t.Fatal("stale bind accepted for changed scope")
				}
			}
			db, err := sql.Open("sqlite", "file:"+filepath.Join(root, "state.db")+"?mode=ro")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var enabled, generation int
			if err = db.QueryRow(`SELECT enabled,generation FROM github_task_binding`).Scan(&enabled, &generation); err != nil || enabled != 1 || generation != 1 {
				t.Fatal("control read or rejected bind revoked running binding", enabled, generation, err)
			}
			// The actual execution boundary reconciles nil/changed config before
			// discovering a deliberately missing dependency and before publishing core.
			previous := a.core.Load()
			if err = a.prepareCore(ctx); err == nil {
				t.Fatal("missing skill dependency unexpectedly prepared")
			}
			if a.core.Load() != previous {
				t.Fatal("failed preparation published an execution Actor")
			}
			if err = db.QueryRow(`SELECT enabled,generation FROM github_task_binding`).Scan(&enabled, &generation); err != nil || enabled != 0 || generation != 2 {
				t.Fatal("runtime preparation failed to revoke old binding first", enabled, generation, err)
			}
			var count int
			if err = db.QueryRow(`SELECT count(*) FROM github_task_followup`).Scan(&count); err != nil || count != 0 {
				t.Fatal("configured-out source promoted", count, err)
			}
			if err = a.Close(); err != nil {
				t.Fatal(err)
			}
			cfg.GitHub = &original
			a, err = OpenControl(ctx, cfg, "", "")
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()

			binding, err := a.GitHubTaskBinding(ctx)
			if err != nil || binding.State != "stopped" {
				t.Fatal("re-added config revived old owner binding", binding, err)
			}
		})
	}
}
