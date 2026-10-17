package sqlite

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"time"

	"github.com/z-chenhao/erii/internal/artifact"
	"github.com/z-chenhao/erii/internal/event"
	eventsqlite "github.com/z-chenhao/erii/internal/event/sqlite"
)

func artifactEvent(ctx context.Context, tx *sql.Tx, ref, kind string) error {
	e, err := event.New("/erii/artifacts", "ai.erii.artifact."+kind+".v1", ref, map[string]string{"artifact_ref": ref})
	if err != nil {
		return err
	}
	return eventsqlite.Enqueue(ctx, tx, e)
}

// Recover removes only uncommitted objects and bytes behind tombstones. A DB
// write lock excludes live writers across processes; os.Root confines unlink.
func (s *Store) Recover(ctx context.Context) (artifact.CleanupReport, error) {
	report := artifact.CleanupReport{Artifacts: []string{}}
	err := s.transaction(ctx, func(tx *sql.Tx) error {
		// Prove writeability under the same lock as writer/recovery. Never create
		// a startup probe outside it, where another process could collect the file.
		name := "tmp-" + token()
		f, e := s.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return artifact.ErrStorage
		}
		closeErr := f.Close()
		removeErr := s.root.Remove(name)
		if closeErr != nil || removeErr != nil {
			return artifact.ErrStorage
		}
		n, e := s.orphans(ctx, tx, false)
		report.Objects = n
		return e
	})
	return report, err
}
func (s *Store) orphans(ctx context.Context, tx *sql.Tx, dry bool) (int, error) {
	dir, err := s.root.Open(".")
	if err != nil {
		return 0, artifact.ErrStorage
	}
	entries, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		return 0, artifact.ErrStorage
	}
	n := 0
	for _, e := range entries {
		name := e.Name()
		if !validToken(name) && !(strings.HasPrefix(name, "tmp-") && validToken(strings.TrimPrefix(name, "tmp-"))) {
			continue
		}
		var count int
		if tx.QueryRowContext(ctx, `SELECT count(*) FROM artifact WHERE storage_key=? AND deleted_at IS NULL`, name).Scan(&count) != nil {
			return n, artifact.ErrStorage
		}
		if count > 0 {
			continue
		}
		n++
		if !dry && s.root.Remove(name) != nil {
			return n, artifact.ErrStorage
		}
	}
	if !dry && n > 0 {
		err = s.sync()
	}
	return n, err
}

// Cleanup is explicitly dry-run by default at the CLI. Referenced objects and
// active/paused Task outputs cannot become candidates. Recheck under one lock.
func (s *Store) Cleanup(ctx context.Context, a artifact.Scope, dry bool) (artifact.CleanupReport, error) {
	report := artifact.CleanupReport{DryRun: dry, Artifacts: []string{}}
	err := s.transaction(ctx, func(tx *sql.Tx) error {
		if !a.Owner {
			return artifact.ErrNotFound
		}
		if err := s.scope(ctx, tx, a); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT artifact_ref,version FROM artifact a WHERE owner_user_id=? AND owner_agent_id=? AND deleted_at IS NULL AND expires_at<=? AND NOT EXISTS(SELECT 1 FROM artifact_reference r WHERE r.artifact_ref=a.artifact_ref) AND NOT EXISTS(SELECT 1 FROM task t WHERE t.task_id=a.task_id AND (t.status IN ('active','paused') OR coalesce(t.completed_at,t.cancelled_at,t.created_at)+a.expires_at-a.created_at>?)) ORDER BY expires_at,artifact_ref LIMIT ?`, a.UserID, a.AgentID, time.Now().UnixMicro(), time.Now().UnixMicro(), s.Policy.CleanupBatch)
		if err != nil {
			return err
		}
		type candidate struct {
			ref     string
			version int64
		}
		list := []candidate{}
		for rows.Next() {
			var c candidate
			if err = rows.Scan(&c.ref, &c.version); err != nil {
				rows.Close()
				return err
			}
			list = append(list, c)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, c := range list {
			report.Artifacts = append(report.Artifacts, c.ref)
			if !dry {
				if err = s.tombstone(ctx, tx, c.ref, c.version); err != nil {
					return err
				}
			}
		}
		// Physical unlink happens only after committing tombstones. Recovery is
		// idempotent if the process stops between these two stages.
		if dry {
			report.Objects, err = s.orphans(ctx, tx, true)
		}
		return err
	})
	if err != nil || dry {
		return report, err
	}
	recovered, err := s.Recover(ctx)
	report.Objects = recovered.Objects
	return report, err
}
