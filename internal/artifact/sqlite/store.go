// Package sqlite stores metadata in state.db and immutable binary objects under
// an os.Root. The caller owns the DB and transactions shared with Tool results.
package sqlite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/z-chenhao/erii/internal/artifact"
)

type Store struct {
	db       *sql.DB
	root     *os.Root
	Policy   artifact.Policy // Configure once before accepting work.
	Boundary func(string)    // Explicit deterministic fault injection only.
}

func New(db *sql.DB, dataRoot string) (*Store, error) {
	if !filepath.IsAbs(dataRoot) {
		return nil, artifact.ErrInvalid
	}
	path := filepath.Join(dataRoot, "artifacts")
	if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, artifact.ErrStorage
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, artifact.ErrStorage
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, artifact.ErrStorage
	}
	return &Store{db: db, root: root, Policy: artifact.DefaultPolicy()}, nil
}
func (s *Store) Close() error { return s.root.Close() }
func (s *Store) boundary(name string) {
	if s.Boundary != nil {
		s.Boundary(name)
	}
}
func token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func validToken(v string) bool {
	b, err := hex.DecodeString(v)
	return len(v) == 64 && err == nil && len(b) == 32 && strings.ToLower(v) == v
}
func validRef(v string) bool {
	return strings.HasPrefix(v, "artifact:") && validToken(strings.TrimPrefix(v, "artifact:"))
}
func validInput(in artifact.Input) bool {
	_, _, err := mime.ParseMediaType(in.MediaType)
	return err == nil && len(in.MediaType) <= 128 && in.Name != "" && in.Name != "." && in.Name != ".." && len(in.Name) <= 255 && !strings.ContainsAny(in.Name, "/\\") && !strings.ContainsFunc(in.Name, unicode.IsControl)
}
func (s *Store) transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return artifact.ErrStorage
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return err
	}
	if tx.Commit() != nil {
		return artifact.ErrStorage
	}
	return nil
}
func (s *Store) scope(ctx context.Context, tx *sql.Tx, a artifact.Scope) error {
	if a.UserID != artifact.LocalUser || a.AgentID == "" || (!a.Owner && a.RunID == "") {
		return artifact.ErrNotFound
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM agent_config WHERE agent_id=?`, a.AgentID).Scan(&n); err != nil {
		return artifact.ErrStorage
	}
	if n != 1 {
		return artifact.ErrNotFound
	}
	if a.RunID != "" {
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM run WHERE run_id=? AND agent_id=? AND coalesce(task_id,'')=?`, a.RunID, a.AgentID, a.TaskID).Scan(&n); err != nil {
			return artifact.ErrStorage
		}
		if n != 1 {
			return artifact.ErrNotFound
		}
	} else if a.TaskID != "" {
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM task WHERE task_id=? AND agent_id=?`, a.TaskID, a.AgentID).Scan(&n); err != nil {
			return artifact.ErrStorage
		}
		if n != 1 {
			return artifact.ErrNotFound
		}
	}
	return nil
}
func (s *Store) DescribeTx(ctx context.Context, tx *sql.Tx, a artifact.Scope, ref string, live bool) (artifact.Descriptor, error) {
	var d artifact.Descriptor
	if !validRef(ref) {
		return d, artifact.ErrNotFound
	}
	if err := s.scope(ctx, tx, a); err != nil {
		return d, err
	}
	var user, agentID, taskID, runID, visibility string
	var created, expires int64
	var deleted sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT artifact_ref,name,media_type,byte_size,content_hash,created_at,expires_at,deleted_at,version,owner_user_id,owner_agent_id,coalesce(task_id,''),coalesce(run_id,''),visibility FROM artifact WHERE artifact_ref=?`, ref).Scan(&d.Ref, &d.Name, &d.MediaType, &d.ByteSize, &d.ContentHash, &created, &expires, &deleted, &d.Version, &user, &agentID, &taskID, &runID, &visibility)
	if err == sql.ErrNoRows {
		return d, artifact.ErrNotFound
	}
	if err != nil {
		return d, artifact.ErrStorage
	}
	if user != a.UserID || agentID != a.AgentID {
		return artifact.Descriptor{}, artifact.ErrNotFound
	}
	visible := a.Owner || (visibility == "task" && taskID != "" && taskID == a.TaskID) || (visibility == "run" && runID == a.RunID)
	if !visible {
		var n int
		err = tx.QueryRowContext(ctx, `SELECT count(*) FROM artifact_reference WHERE artifact_ref=? AND ((holder_kind='task' AND holder_id=?) OR (holder_kind='run' AND holder_id=?))`, ref, a.TaskID, a.RunID).Scan(&n)
		if err != nil {
			return d, artifact.ErrStorage
		}
		visible = n > 0
	}
	// Task-owned and explicitly attached inputs remain recoverable for the
	// complete Task lifecycle, then retain their creation-time TTL after it ends.
	protected, effective, err := s.expiry(ctx, tx, ref, taskID, created, expires)
	if err != nil {
		return artifact.Descriptor{}, err
	}
	expires = effective
	if !visible || ((live || !a.Owner) && (deleted.Valid || (!protected && expires <= time.Now().UnixMicro()))) {
		return artifact.Descriptor{}, artifact.ErrNotFound
	}
	d.CreatedAt = time.UnixMicro(created).UTC()
	if !protected {
		at := time.UnixMicro(expires).UTC()
		d.ExpiresAt = &at
	}
	if deleted.Valid {
		v := time.UnixMicro(deleted.Int64).UTC()
		d.DeletedAt = &v
	}
	return d, nil
}
func (s *Store) expiry(ctx context.Context, tx *sql.Tx, ref, taskID string, created, expires int64) (bool, int64, error) {
	var active int
	var ended int64
	err := tx.QueryRowContext(ctx, `SELECT coalesce(max(status IN ('active','paused')),0),coalesce(max(coalesce(completed_at,cancelled_at,created_at)),0) FROM task t WHERE t.task_id=? OR EXISTS(SELECT 1 FROM artifact_reference r WHERE r.artifact_ref=? AND r.holder_kind='task' AND r.holder_id=t.task_id)`, taskID, ref).Scan(&active, &ended)
	if err != nil {
		return false, 0, artifact.ErrStorage
	}
	if ended > 0 && ended+expires-created > expires {
		expires = ended + expires - created
	}
	return active > 0, expires, nil
}

func (s *Store) Describe(ctx context.Context, a artifact.Scope, ref string) (d artifact.Descriptor, err error) {
	err = s.transaction(ctx, func(tx *sql.Tx) error { var e error; d, e = s.DescribeTx(ctx, tx, a, ref, false); return e })
	return
}
func (s *Store) Put(ctx context.Context, a artifact.Scope, in artifact.Input, src io.Reader) (ref string, err error) {
	err = s.transaction(ctx, func(tx *sql.Tx) error { var e error; ref, e = s.PutTx(ctx, tx, a, in, src, ""); return e })
	return
}

// PutTx must run inside an immediate state.db transaction. This serializes
// quota admission and orphan recovery across processes, including live writers.
// key is a Host-owned invocation key. Never accept it from a model or user.
func (s *Store) PutTx(ctx context.Context, tx *sql.Tx, a artifact.Scope, in artifact.Input, src io.Reader, key string) (string, error) {
	if src == nil || !validInput(in) || s.Policy.Validate() != nil {
		return "", artifact.ErrInvalid
	}
	if err := s.scope(ctx, tx, a); err != nil {
		return "", err
	}
	obj := token()
	tmp := "tmp-" + obj
	f, err := s.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", artifact.ErrStorage
	}
	defer f.Close()
	defer s.root.Remove(tmp)
	s.boundary("artifact_temp_created")
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(&contextReader{ctx, src}, s.Policy.MaxBytes+1))
	if err != nil {
		return "", artifact.ErrStorage
	}
	if n > s.Policy.MaxBytes {
		return "", artifact.ErrLimit
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	if key != "" {
		var ref, oldHash, name, media, runID string
		err = tx.QueryRowContext(ctx, `SELECT artifact_ref,content_hash,name,media_type,coalesce(run_id,'') FROM artifact WHERE put_key=?`, key).Scan(&ref, &oldHash, &name, &media, &runID)
		if err == nil {
			if oldHash != sum || name != in.Name || media != in.MediaType || runID != a.RunID {
				return "", artifact.ErrConflict
			}
			_, err = s.DescribeTx(ctx, tx, a, ref, true)
			return ref, err
		}
		if err != sql.ErrNoRows {
			return "", artifact.ErrStorage
		}
	}
	var used int64
	if tx.QueryRowContext(ctx, `SELECT coalesce(sum(byte_size),0) FROM artifact WHERE owner_user_id=? AND deleted_at IS NULL`, a.UserID).Scan(&used) != nil {
		return "", artifact.ErrStorage
	}
	if n > s.Policy.QuotaBytes-used {
		return "", artifact.ErrLimit
	}
	if f.Chmod(0400) != nil || f.Sync() != nil || f.Close() != nil {
		return "", artifact.ErrStorage
	}
	// Verify the actual written object, not only the source stream.
	if err = s.verify(tmp, n, sum); err != nil {
		return "", err
	}
	s.boundary("artifact_before_publish")
	if s.root.Rename(tmp, obj) != nil || s.sync() != nil {
		return "", artifact.ErrStorage
	}
	s.boundary("artifact_after_publish")
	ref := "artifact:" + token()
	id := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC()
	visibility := "agent"
	if a.TaskID != "" {
		visibility = "task"
	} else if a.RunID != "" {
		visibility = "run"
	}
	var putKey any
	if key != "" {
		putKey = key
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO artifact(artifact_id,artifact_ref,owner_user_id,owner_agent_id,task_id,run_id,media_type,name,byte_size,content_hash,storage_key,visibility,retention_class,created_at,expires_at,put_key) VALUES(?,?,?,?,nullif(?,''),nullif(?,''),?,?,?,?,?,?,'standard',?,?,?)`, id, ref, a.UserID, a.AgentID, a.TaskID, a.RunID, in.MediaType, in.Name, n, sum, obj, visibility, now.UnixMicro(), now.Add(time.Duration(s.Policy.RetentionDays)*24*time.Hour).UnixMicro(), putKey)
	if err != nil {
		return "", artifact.ErrStorage
	}
	if err = artifactEvent(ctx, tx, ref, "created"); err != nil {
		return "", err
	}
	s.boundary("artifact_after_metadata")
	return ref, nil
}

type contextReader struct {
	ctx context.Context
	src io.Reader
}

func (r *contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.src.Read(b)
	if cause := r.ctx.Err(); cause != nil {
		return 0, cause
	}
	return n, err
}
func (s *Store) sync() error {
	f, e := s.root.Open(".")
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func (s *Store) file(key string) (*os.File, error) {
	if !validToken(key) && !(strings.HasPrefix(key, "tmp-") && validToken(strings.TrimPrefix(key, "tmp-"))) {
		return nil, artifact.ErrIntegrity
	}
	info, err := s.root.Lstat(key)
	if err != nil || !info.Mode().IsRegular() {
		return nil, artifact.ErrIntegrity
	}
	f, err := s.root.Open(key)
	if err != nil {
		return nil, artifact.ErrIntegrity
	}
	return f, nil
}
func (s *Store) verify(key string, n int64, sum string) error {
	f, err := s.file(key)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() != n {
		return artifact.ErrIntegrity
	}
	h := sha256.New()
	if _, err = io.Copy(h, io.LimitReader(f, n+1)); err != nil || hex.EncodeToString(h.Sum(nil)) != sum {
		return artifact.ErrIntegrity
	}
	return nil
}
func (s *Store) OpenTx(ctx context.Context, tx *sql.Tx, a artifact.Scope, ref string) (io.ReadCloser, error) {
	d, err := s.DescribeTx(ctx, tx, a, ref, true)
	if err != nil {
		return nil, err
	}
	var key string
	if tx.QueryRowContext(ctx, `SELECT storage_key FROM artifact WHERE artifact_ref=?`, ref).Scan(&key) != nil {
		return nil, artifact.ErrStorage
	}
	// Return the same verified descriptor to avoid a pathname swap after hashing.
	f, err := s.file(key)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(&contextReader{ctx, f}, d.ByteSize+1))
	if err != nil || n != d.ByteSize || hex.EncodeToString(h.Sum(nil)) != d.ContentHash {
		f.Close()
		return nil, artifact.ErrIntegrity
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, artifact.ErrStorage
	}
	return f, nil
}
func (s *Store) Open(ctx context.Context, a artifact.Scope, ref string) (f io.ReadCloser, err error) {
	err = s.transaction(ctx, func(tx *sql.Tx) error { var e error; f, e = s.OpenTx(ctx, tx, a, ref); return e })
	if err != nil && f != nil {
		f.Close()
	}
	return
}
func (s *Store) PinTx(ctx context.Context, tx *sql.Tx, a artifact.Scope, ref, kind, id string) error {
	if _, err := s.DescribeTx(ctx, tx, a, ref, true); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO artifact_reference VALUES(?,?,?) ON CONFLICT DO NOTHING`, ref, kind, id)
	return err
}

// PublishTx validates the complete immutable set, including content integrity,
// before any mutation. Expected validation failures leave the transaction clean.
func (s *Store) PublishTx(ctx context.Context, tx *sql.Tx, a artifact.Scope, refs []string) error {
	if a.RunID == "" {
		return artifact.ErrInvalid
	}
	if len(refs) > artifact.MaxPublished {
		return artifact.ErrLimit
	}
	if err := s.scope(ctx, tx, a); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT artifact_ref FROM artifact_publication WHERE run_id=?`, a.RunID)
	if err != nil {
		return artifact.ErrStorage
	}
	selected := map[string]bool{}
	for rows.Next() {
		var ref string
		if err = rows.Scan(&ref); err != nil {
			rows.Close()
			return artifact.ErrStorage
		}
		selected[ref] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return artifact.ErrStorage
	}
	for _, ref := range refs {
		selected[ref] = true
	}
	if len(selected) > artifact.MaxPublished {
		return artifact.ErrLimit
	}
	for _, ref := range refs {
		f, err := s.OpenTx(ctx, tx, a, ref)
		if err != nil {
			return err
		}
		if f.Close() != nil {
			return artifact.ErrStorage
		}
	}
	for _, ref := range refs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO artifact_reference VALUES(?,'publication',?) ON CONFLICT DO NOTHING`, ref, a.RunID); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO artifact_publication VALUES(?,?) ON CONFLICT DO NOTHING`, a.RunID, ref)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			if err = artifactEvent(ctx, tx, ref, "published"); err != nil {
				return err
			}
		}
	}
	return nil
}
func (s *Store) ListTx(ctx context.Context, tx *sql.Tx, a artifact.Scope) ([]artifact.Descriptor, error) {
	if err := s.scope(ctx, tx, a); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT artifact_ref FROM artifact WHERE owner_user_id=? AND owner_agent_id=? AND deleted_at IS NULL ORDER BY created_at DESC,artifact_ref`, a.UserID, a.AgentID)
	if err != nil {
		return nil, err
	}
	refs := []string{}
	for rows.Next() {
		var ref string
		if err = rows.Scan(&ref); err != nil {
			rows.Close()
			return nil, err
		}
		refs = append(refs, ref)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []artifact.Descriptor{}
	for _, ref := range refs {
		d, e := s.DescribeTx(ctx, tx, a, ref, true)
		if errors.Is(e, artifact.ErrNotFound) {
			continue
		}
		if e != nil {
			return nil, e
		}
		out = append(out, d)
		if len(out) == 50 {
			break
		}
	}
	return out, nil
}
func (s *Store) Delete(ctx context.Context, a artifact.Scope, ref string, version int64) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		d, err := s.DescribeTx(ctx, tx, a, ref, false)
		if err != nil {
			return err
		}
		if !a.Owner {
			return artifact.ErrNotFound
		}
		if d.Version != version {
			return artifact.ErrConflict
		}
		if d.DeletedAt != nil {
			return nil
		}
		var n int
		if tx.QueryRowContext(ctx, `SELECT count(*) FROM artifact_reference WHERE artifact_ref=?`, ref).Scan(&n) != nil {
			return artifact.ErrStorage
		}
		if n > 0 {
			return artifact.ErrReferenced
		}
		return s.tombstone(ctx, tx, ref, version)
	})
}
func (s *Store) tombstone(ctx context.Context, tx *sql.Tx, ref string, version int64) error {
	res, err := tx.ExecContext(ctx, `UPDATE artifact SET deleted_at=?,version=version+1 WHERE artifact_ref=? AND version=? AND deleted_at IS NULL`, time.Now().UnixMicro(), ref, version)
	if err != nil {
		return artifact.ErrStorage
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return artifact.ErrConflict
	}
	return artifactEvent(ctx, tx, ref, "deleted")
}
