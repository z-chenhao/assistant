package secret

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestAccountFilesRoundTrip(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := AccountFiles{DataRoot: root}
	id := uuid.NewString()
	ref := "credential:" + id
	path := filepath.Join(root, "account-secrets", id)
	if exists, err := store.Exists(ctx, ref); err != nil || exists {
		t.Fatalf("unexpected credential before write: exists=%v err=%v", exists, err)
	}
	for _, value := range []string{"synthetic-initial", "synthetic-rotated"} {
		if err := store.Put(ctx, ref, []byte(value)); err != nil {
			t.Fatal(err)
		}
		reopened := AccountFiles{DataRoot: root}
		got, err := reopened.Get(ctx, ref)
		if err != nil || string(got) != value {
			t.Fatalf("credential did not survive reopening: %v", err)
		}
	}
	for name, mode := range map[string]os.FileMode{path: 0600, filepath.Dir(path): 0700} {
		info, err := os.Stat(name)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("unexpected credential permissions: %v", err)
		}
	}
	if err := store.Delete(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if exists, err := store.Exists(ctx, ref); err != nil || exists {
		t.Fatalf("credential survived deletion: exists=%v err=%v", exists, err)
	}
}

func TestAccountFilesRejectsSymlinkAndUnsafeReference(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := AccountFiles{DataRoot: root}
	id := uuid.NewString()
	ref := "credential:" + id
	if err := store.Put(ctx, "credential:../outside", []byte("synthetic")); err == nil {
		t.Fatal("accepted a path outside the credential store")
	}
	dir := filepath.Join(root, "account-secrets")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("synthetic-other-item"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, id)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, ref); err == nil {
		t.Fatal("read through a credential symlink")
	}
	if err := store.Put(ctx, ref, []byte("synthetic-replacement")); err != nil {
		t.Fatal(err)
	}
	value, err := os.ReadFile(outside)
	if err != nil || string(value) != "synthetic-other-item" {
		t.Fatalf("writing the credential changed another file: %v", err)
	}
}
