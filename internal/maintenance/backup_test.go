package maintenance_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"github.com/z-chenhao/erii/internal/app"
	"github.com/z-chenhao/erii/internal/config"
	"github.com/z-chenhao/erii/internal/maintenance"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func fixture(t *testing.T) (string, []byte) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "data")
	cfg, e := config.Load("", func(k string) (string, bool) { return root, k == "ERII_DATA_ROOT" })
	if e != nil {
		t.Fatal(e)
	}
	a, e := app.Open(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.Close(); e != nil {
		t.Fatal(e)
	}
	return root, bytes.Repeat([]byte{91}, 32)
}

func TestEncryptedOfflineRoundtripAndFailClosed(t *testing.T) {
	ctx := context.Background()
	root, key := fixture(t)
	max := app.SchemaVersions()
	archive := filepath.Join(t.TempDir(), "backup.erii")
	lock, e := maintenance.Lock(root, false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = maintenance.Backup(ctx, root, archive, key, max); !errors.Is(e, maintenance.ErrBusy) {
		t.Fatal("live root accepted", e)
	}
	lock.Close()
	manifest, e := maintenance.Backup(ctx, root, archive, key, max)
	if e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(archive)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(data, []byte("SQLite format 3")) || bytes.Contains(data, []byte("agent_config")) {
		t.Fatal("plaintext archive")
	}
	if _, e = maintenance.Backup(ctx, root, archive, key, max); e == nil {
		t.Fatal("overwrote archive")
	}
	for name, raw := range map[string][]byte{"wrong-key": data, "truncated": data[:len(data)-1], "appended": append(append([]byte{}, data...), 1), "corrupted": append([]byte{}, data...)} {
		t.Run(name, func(t *testing.T) {
			k := key
			if name == "wrong-key" {
				k = bytes.Repeat([]byte{92}, 32)
			}
			if name == "corrupted" {
				raw[len(raw)/2] ^= 1
			}
			path := filepath.Join(t.TempDir(), "bad.erii")
			if e = os.WriteFile(path, raw, 0600); e != nil {
				t.Fatal(e)
			}
			target := filepath.Join(t.TempDir(), "restore")
			if _, e = maintenance.Restore(ctx, path, target, k, max); e == nil {
				t.Fatal("bad archive accepted")
			}
			if _, e = os.Stat(target); !os.IsNotExist(e) {
				t.Fatal("failed restore published state")
			}
		})
	}
	target := filepath.Join(t.TempDir(), "restore")
	restored, e := maintenance.Restore(ctx, archive, target, key, max)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(manifest, restored) || !maintenance.Pending(target) {
		t.Fatal("lost manifest or restore fence")
	}
	if _, e = maintenance.Restore(ctx, archive, target, key, max); e == nil {
		t.Fatal("overwrote target")
	}
	if e = maintenance.Activate(target, "yes"); e == nil {
		t.Fatal("implicit activation")
	}
	if e = maintenance.Activate(target, "source-retired-and-outcomes-reconciled"); e != nil {
		t.Fatal(e)
	}
	if maintenance.Pending(target) {
		t.Fatal("fence not removed")
	}
	for _, entry := range manifest.Files {
		actual, e := os.ReadFile(filepath.Join(target, entry.Path))
		if e != nil {
			t.Fatal(e)
		}
		before, e := os.ReadFile(filepath.Join(root, entry.Path))
		if e != nil || !bytes.Equal(actual, before) {
			t.Fatal("bytes changed", entry.Path, e)
		}
	}
}

func TestAllDatabaseVersionPreflightBeforeAnyMigration(t *testing.T) {
	ctx := context.Background()
	root, _ := fixture(t)
	for name, version := range map[string]int{"state.db": 25, "memory.db": 999} {
		db, e := sql.Open("sqlite", filepath.Join(root, name))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec("PRAGMA user_version=" + map[int]string{25: "25", 999: "999"}[version]); e != nil {
			t.Fatal(e)
		}
		db.Close()
	}
	cfg, e := config.Load("", func(k string) (string, bool) { return root, k == "ERII_DATA_ROOT" })
	if e != nil {
		t.Fatal(e)
	}
	if _, e = app.Open(ctx, cfg); !errors.Is(e, maintenance.ErrInvalid) {
		t.Fatal("future schema accepted", e)
	}
	db, e := sql.Open("sqlite", filepath.Join(root, "state.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var v int
	if db.QueryRow("PRAGMA user_version").Scan(&v) != nil || v != 25 {
		t.Fatal("migrated state before rejecting future Memory", v)
	}
}

func TestBackupRefusesSymlinksAndUnverifiedArtifacts(t *testing.T) {
	root, key := fixture(t)
	outside := filepath.Join(t.TempDir(), "private")
	if e := os.WriteFile(outside, []byte("SECRET_SENTINEL"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(outside, filepath.Join(root, "artifacts", "escaped")); e != nil {
		t.Fatal(e)
	}
	if _, e := maintenance.Backup(context.Background(), root, filepath.Join(t.TempDir(), "backup"), key, app.SchemaVersions()); e == nil {
		t.Fatal("symlink archived")
	}
}
