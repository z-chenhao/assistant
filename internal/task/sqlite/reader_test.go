package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestTaskWorkspacePoolCannotCreateOrWriteState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	reader, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.db.PingContext(context.Background()); err == nil {
		t.Fatal("task browsing created a database")
	}
	reader.Close()
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("read created state", err)
	}
	writer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err = writer.Exec(`CREATE TABLE value(text TEXT);INSERT INTO value VALUES('original')`); err != nil {
		t.Fatal(err)
	}
	reader, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if _, err = reader.db.Exec(`UPDATE value SET text='modified'`); err == nil {
		t.Fatal("task reader can mutate state")
	}
	var value string
	if err = reader.db.QueryRow(`SELECT text FROM value`).Scan(&value); err != nil || value != "original" {
		t.Fatal(value, err)
	}
}
