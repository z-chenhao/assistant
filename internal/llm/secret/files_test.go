package secret

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/z-chenhao/assistant/internal/memory"
)

func TestFilesKeepsMemoryKeyAcrossReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	ref := "credential:memory-test-key"
	first := Files{DataRoot: root}
	key := bytes.Repeat([]byte{71}, 32)
	if err := first.Put(ctx, ref, key); err != nil {
		t.Fatal(err)
	}
	vault := memory.Vault{Secrets: first, Ref: ref}
	sealed, err := vault.Seal(ctx, "private memory", "test-binding")
	if err != nil {
		t.Fatal(err)
	}
	reopened := Files{DataRoot: root}
	plain, err := (memory.Vault{Secrets: reopened, Ref: ref}).Open(ctx, sealed, "test-binding")
	if err != nil || plain != "private memory" {
		t.Fatal("key did not survive reopening", err)
	}
	for name, mode := range map[string]os.FileMode{"secrets": 0700, "secrets/memory-test-key": 0600} {
		info, err := os.Stat(filepath.Join(root, name))
		if err != nil || info.Mode().Perm() != mode {
			t.Fatal("unsafe secret permissions", name, err)
		}
	}
	t.Setenv("STEWARD_FILE_SECRET_TEST", "synthetic-env-value")
	value, err := reopened.Get(ctx, "env:STEWARD_FILE_SECRET_TEST")
	if err != nil || string(value) != "synthetic-env-value" {
		t.Fatal("explicit environment reference failed", err)
	}
	if err := reopened.Delete(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Get(ctx, ref); err == nil {
		t.Fatal("deleted secret still readable")
	}
}

func TestFilesRejectsUnsafePathsAndPermissions(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := Files{DataRoot: root}
	for _, ref := range []string{"credential:../escape", "credential:.", "credential:a/b", "credential:", "file:/tmp/key"} {
		if err := store.Put(ctx, ref, []byte("synthetic")); err == nil {
			t.Fatal("unsafe reference accepted", ref)
		}
	}
	if err := store.Put(ctx, "credential:key", []byte("synthetic")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "secrets/key")
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "credential:key"); err == nil {
		t.Fatal("read a publicly readable secret")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "credential:key"); err == nil {
		t.Fatal("read through symlink")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "credential:key"); err == nil {
		t.Fatal("read through hard link")
	}
	if err := os.Chmod(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, "credential:other", []byte("synthetic")); err == nil {
		t.Fatal("wrote in shared directory")
	}
}
