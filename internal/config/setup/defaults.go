package setup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/z-chenhao/assistant/internal/computer/macosvm"
	"github.com/z-chenhao/assistant/internal/config"
	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/memory"
	"github.com/z-chenhao/assistant/internal/skill"
	"golang.org/x/sys/unix"
)

// PrepareDefaults runs only at application startup, never on a settings read or
// doctor. The owner document retains automatic mode; effective values stay local.
func (m *Manager) PrepareDefaults(ctx context.Context) config.Config {
	c := m.active
	m.states = map[string]string{}
	if c.Automatic("memory") {
		ref, err := defaultMemoryKey(ctx, c.App.DataRoot, m.secrets)
		if err == nil {
			c.Memory = &memory.Config{EncryptionKeyRef: ref}
			m.states["memory"] = "ready"
		} else {
			m.states["memory"] = "unavailable"
		}
	}
	if c.Automatic("skills") {
		root := c.App.DataRoot + ".skills"
		if os.MkdirAll(root, 0700) == nil {
			candidate := &skill.Config{Root: root, Scope: "agent"}
			if _, err := skill.Definitions(*candidate); err == nil {
				c.Skills = candidate
				m.states["skills"] = "ready"
			} else {
				m.states["skills"] = "unavailable"
			}
		} else {
			m.states["skills"] = "unavailable"
		}
	}
	if c.Automatic("embedding") {
		m.states["embedding"] = "ready"
	}
	if c.Automatic("computer") && runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		managed := macosvm.DefaultConfig(filepath.Join(c.App.DataRoot+".desktop", "store"))
		c.Computer = &managed
		c.Execution = nil
	}
	return c
}

// A random reference is published only after SecretStore confirms the key. A
// crash can orphan a newly allocated item, but never replaces an existing key.
// A published reference that is locked/missing is never silently regenerated.
func defaultMemoryKey(ctx context.Context, root string, secrets llm.SecretStore) (string, error) {
	if secrets == nil {
		return "", llm.ErrSecretStore
	}
	fd, err := unix.Open(filepath.Join(root, ".defaults.lock"), unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return "", llm.ErrSecretStore
	}
	lock := os.NewFile(uintptr(fd), "defaults lock")
	defer lock.Close()
	for unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB) != nil {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	defer unix.Flock(fd, unix.LOCK_UN)
	dir, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer dir.Close()
	const name = "default-memory.json"
	var saved struct {
		Ref string `json:"ref"`
	}
	info, err := dir.Lstat(name)
	if err == nil {
		if !info.Mode().IsRegular() || info.Size() > 1024 {
			return "", llm.ErrSecretStore
		}
		raw, e := dir.ReadFile(name)
		if e != nil || len(raw) > 1024 || json.Unmarshal(raw, &saved) != nil || saved.Ref == "" {
			return "", llm.ErrSecretStore
		}
		key, e := secrets.Get(ctx, saved.Ref)
		defer clear(key)
		if e != nil || len(key) != 32 {
			return "", llm.ErrSecretStore
		}
		return saved.Ref, nil
	}
	if !os.IsNotExist(err) {
		return "", llm.ErrSecretStore
	}
	if info, err := os.Lstat(filepath.Join(root, "memory.db")); err == nil {
		if !info.Mode().IsRegular() {
			return "", llm.ErrSecretStore
		}
		u := url.URL{Scheme: "file", Path: filepath.Join(root, "memory.db")}
		u.RawQuery = "mode=ro"
		db, err := sql.Open("sqlite", u.String())
		if err != nil {
			return "", llm.ErrSecretStore
		}
		defer db.Close()
		var encrypted bool
		if db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM memory_record WHERE content LIKE 'sealed:v1:%')`).Scan(&encrypted) != nil || encrypted {
			return "", llm.ErrSecretStore
		}
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return "", err
	}
	defer clear(key)
	id := sha256.Sum256(key)
	saved.Ref = "credential:memory-" + hex.EncodeToString(id[:16])
	if err = secrets.Put(ctx, saved.Ref, key); err != nil {
		return "", err
	}
	check, err := secrets.Get(ctx, saved.Ref)
	defer clear(check)
	if err != nil || len(check) != 32 || sha256.Sum256(check) != sha256.Sum256(key) {
		return "", llm.ErrSecretStore
	}
	raw, _ := json.Marshal(saved)
	tmp := ".default-memory-" + hex.EncodeToString(id[:8])
	f, err := dir.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err = dir.Rename(tmp, name); err != nil {
		return "", err
	}
	d, err := dir.Open(".")
	if err != nil {
		return "", err
	}
	defer d.Close()
	if err = d.Sync(); err != nil {
		return "", err
	}
	return saved.Ref, nil
}

func savedMemoryRef(root string) string {
	var saved struct {
		Ref string `json:"ref"`
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return ""
	}
	defer dir.Close()
	info, err := dir.Lstat("default-memory.json")
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1024 {
		return ""
	}
	raw, err := dir.ReadFile("default-memory.json")
	if err != nil || json.Unmarshal(raw, &saved) != nil {
		return ""
	}
	return saved.Ref
}
