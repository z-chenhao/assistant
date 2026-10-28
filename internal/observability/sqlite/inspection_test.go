package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectionConnectionEnforcesReadOnlyAndDoesNotCreateState(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	missing, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = missing.db.PingContext(ctx); err == nil {
		missing.Close()
		t.Fatal("inspection created missing state")
	}
	missing.Close()
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("inspection created state on read", err)
	}
	writer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err = writer.Exec(`CREATE TABLE probe(value TEXT);INSERT INTO probe VALUES('original')`); err != nil {
		t.Fatal(err)
	}
	reader, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if _, err = reader.db.Exec(`UPDATE probe SET value='changed'`); err == nil {
		t.Fatal("inspector connection could mutate state")
	}
	var value string
	if err = reader.db.QueryRow(`SELECT value FROM probe`).Scan(&value); err != nil || value != "original" {
		t.Fatal(value, err)
	}
}
