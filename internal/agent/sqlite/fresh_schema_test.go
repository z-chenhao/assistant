package sqlite

import (
	"bufio"
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var updateStateSchema = flag.Bool("update-state-schema", false, "regenerate the derived empty state.db schema")

type schemaObject struct{ Type, Name, Table, SQL string }

func schemaObjects(t *testing.T, db *sql.DB, order string) []schemaObject {
	t.Helper()
	rows, err := db.Query(`SELECT type,name,tbl_name,coalesce(sql,'') FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%' ORDER BY ` + order)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var objects []schemaObject
	for rows.Next() {
		var object schemaObject
		if err := rows.Scan(&object.Type, &object.Name, &object.Table, &object.SQL); err != nil {
			t.Fatal(err)
		}
		objects = append(objects, object)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return objects
}

func schemaDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestFreshSchemaSnapshotMatchesHistory(t *testing.T) {
	ctx := context.Background()
	history := schemaDB(t)
	tx, err := history.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	// Execute the authoritative historical scripts directly, independently
	// of migrate's fresh-database optimization.
	for _, script := range migrations[:freshSchemaVersion] {
		if _, err := tx.ExecContext(ctx, script); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var generated strings.Builder
	generated.WriteString(freshSchemaHeader(migrations[:freshSchemaVersion]))
	for _, object := range schemaObjects(t, history, "rowid") {
		fmt.Fprintln(&generated, object.SQL+";")
	}
	if *updateStateSchema {
		if err := os.WriteFile("schema.sql", []byte(generated.String()), 0644); err != nil {
			t.Fatal(err)
		}
		return // The embedded bytes belong to the preceding build; re-run to verify.
	}
	if freshSchemaSQL != generated.String() {
		t.Fatal("derived empty schema drifted; regenerate with -update-state-schema")
	}
	fresh := schemaDB(t)
	if err := migrate(ctx, fresh, migrations[:freshSchemaVersion]); err != nil {
		t.Fatal(err)
	}
	if got, want := schemaObjects(t, fresh, "type,name"), schemaObjects(t, history, "type,name"); !reflect.DeepEqual(got, want) {
		t.Fatal("fresh schema differs from the historical tables, indexes or triggers")
	}
	for _, object := range schemaObjects(t, history, "name") {
		if object.Type != "table" {
			continue
		}
		var oldCount, newCount int
		name := `"` + strings.ReplaceAll(object.Name, `"`, `""`) + `"`
		if err := history.QueryRow("SELECT count(*) FROM " + name).Scan(&oldCount); err != nil {
			t.Fatal(err)
		}
		if err := fresh.QueryRow("SELECT count(*) FROM " + name).Scan(&newCount); err != nil {
			t.Fatal(err)
		}
		if oldCount != 0 || newCount != oldCount {
			t.Fatalf("initial data differs for %s: %d -> %d; a schema-only snapshot cannot supply seed data", object.Name, oldCount, newCount)
		}
	}
}

func TestFreshSchemaAndLaterMigrationAreAtomic(t *testing.T) {
	ctx := context.Background()
	db := schemaDB(t)
	scripts := append(append([]string{}, migrations...), "CREATE TABLE later(value TEXT); INSERT INTO later VALUES('kept'); INVALID SQL")
	if err := migrate(ctx, db, scripts); err == nil {
		t.Fatal("accepted failed migration after fresh schema creation")
	}
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 0 {
		t.Fatal("failed initialization advanced schema version", version, err)
	}
	if objects := schemaObjects(t, db, "name"); len(objects) != 0 {
		t.Fatal("failed initialization retained partial schema", objects)
	}
	scripts[len(scripts)-1] = "CREATE TABLE later(value TEXT); INSERT INTO later VALUES('kept');"
	if err := migrate(ctx, db, scripts); err != nil {
		t.Fatal(err)
	}
	var value string
	if err := db.QueryRow("SELECT value FROM later").Scan(&value); err != nil || value != "kept" {
		t.Fatal("later migration lost its initial data", value, err)
	}
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != len(scripts) {
		t.Fatal("later migration version", version, err)
	}
}

func TestFreshSchemaRejectsChangedHistory(t *testing.T) {
	db := schemaDB(t)
	scripts := append([]string{}, migrations...)
	scripts[0] += "\n-- changed history\n"
	if err := migrate(context.Background(), db, scripts); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatal("accepted stale schema snapshot", err)
	}
	if objects := schemaObjects(t, db, "name"); len(objects) != 0 {
		t.Fatal("rejected snapshot changed database", objects)
	}
}

func TestFreshSchemaCollisionPreservesUnversionedData(t *testing.T) {
	db := schemaDB(t)
	if _, err := db.Exec("CREATE TABLE run(obstruction TEXT); INSERT INTO run VALUES('preserved')"); err != nil {
		t.Fatal(err)
	}
	before := schemaObjects(t, db, "name")
	if err := migrate(context.Background(), db, migrations); err == nil {
		t.Fatal("overwrote an incompatible unversioned database")
	}
	var version int
	var value string
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 0 {
		t.Fatal("collision advanced schema version", version, err)
	}
	if err := db.QueryRow("SELECT obstruction FROM run").Scan(&value); err != nil || value != "preserved" {
		t.Fatal("collision changed preexisting data", value, err)
	}
	if got := schemaObjects(t, db, "name"); !reflect.DeepEqual(got, before) {
		t.Fatal("collision retained partially initialized schema", got)
	}
}

func TestFreshSchemaCrashHelper(t *testing.T) {
	root := os.Getenv("COLLIE_TEST_SCHEMA_CRASH_ROOT")
	if root == "" {
		return
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "state.db")+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(freshSchemaSQL); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version=%d", freshSchemaVersion)); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("COLLIE_TEST_SCHEMA_CRASH_PHASE") == "after_commit" {
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("schema-crash-ready")
	for {
		time.Sleep(time.Minute) // The parent kills this exact held fixture process.
	}
}

func TestFreshSchemaKillRestart(t *testing.T) {
	for _, phase := range []string{"before_commit", "after_commit"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFreshSchemaCrashHelper$")
			child.Env = append(os.Environ(), "COLLIE_TEST_SCHEMA_CRASH_ROOT="+root, "COLLIE_TEST_SCHEMA_CRASH_PHASE="+phase)
			output, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			child.Stderr = os.Stderr
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			// Cleanup also owns the child on every readiness/error path.
			defer func() {
				if child.ProcessState == nil {
					_ = child.Process.Kill()
					_ = child.Wait()
				}
			}()
			scanner := bufio.NewScanner(output)
			if !scanner.Scan() || scanner.Text() != "schema-crash-ready" {
				t.Fatal("schema fixture did not reach held commit boundary", scanner.Err())
			}
			if err := child.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			if err := child.Wait(); err == nil {
				t.Fatal("schema fixture exited successfully instead of being killed")
			}
			db, err := sql.Open("sqlite", filepath.Join(root, "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var version int
			if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
				_ = db.Close()
				t.Fatal(err)
			}
			want := 0
			if phase == "after_commit" {
				want = freshSchemaVersion
			}
			if version != want || (want == 0 && len(schemaObjects(t, db, "name")) != 0) {
				_ = db.Close()
				t.Fatal("crash retained partial schema/version", version, want)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			s, err := Open(context.Background(), root)
			if err != nil {
				t.Fatal("restart after schema crash", err)
			}
			defer s.Close()
			d, err := s.Diagnostics(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if d.SchemaVersion != len(migrations) || scalar(t, s, "SELECT count(*) FROM agent_config") != 1 || scalar(t, s, "SELECT count(*) FROM event_outbox WHERE type='ai.collie.agent.initialized.v1'") != 1 {
				t.Fatal("restart lost schema or duplicated bootstrap", d)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
