package main

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/z-chenhao/assistant/internal/app"
	"github.com/z-chenhao/assistant/internal/config"
	"github.com/z-chenhao/assistant/internal/llm/secret"
)

func TestOfflineRestoreUsesSourceFileKeyAndNewDestination(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	root := filepath.Join(base, "source")
	cfg, err := config.Load("", func(k string) (string, bool) { return root, k == "STEWARD_DATA_ROOT" })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Defaults = &config.Defaults{Memory: true, Search: true}
	a, err := app.OpenControl(ctx, cfg, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	ref := "credential:offline-backup-key"
	store := secret.Files{DataRoot: root}
	if err := store.Put(ctx, ref, bytes.Repeat([]byte{33}, 32)); err != nil {
		t.Fatal(err)
	}
	archive, target := filepath.Join(base, "backup.steward"), filepath.Join(base, "restored")
	invoke := func(args ...string) {
		t.Helper()
		flags := flag.NewFlagSet("ops", flag.ContinueOnError)
		if err := flags.Parse(args); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		handled, err := runOfflineOperation(ctx, cfg, flags, "", &output)
		if !handled || err != nil {
			t.Fatal("offline file-key operation failed", err)
		}
	}
	invoke("ops", "backup", archive, ref)
	invoke("ops", "restore", archive, ref, target)
	if _, err := os.Stat(filepath.Join(target, "state.db")); err != nil {
		t.Fatal("restore destination missing", err)
	}
	if _, err := os.Stat(filepath.Join(target, "secrets")); !os.IsNotExist(err) {
		t.Fatal("restore copied credential files", err)
	}
	if key, err := store.Get(ctx, ref); err != nil || !bytes.Equal(key, bytes.Repeat([]byte{33}, 32)) {
		t.Fatal("restore changed source credentials", err)
	}
}
