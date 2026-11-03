package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestComputerActivityIsBoundedScopedAndOmitsArguments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "computer-activity.db")
	writer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	_, err = writer.Exec(`CREATE TABLE agent_config(singleton INTEGER,agent_id TEXT);INSERT INTO agent_config VALUES(1,'owner');CREATE TABLE run(run_id TEXT,agent_id TEXT);INSERT INTO run VALUES('own-run','owner'),('foreign-run','other');CREATE TABLE tool_invocation(invocation_id TEXT,run_id TEXT,tool_name TEXT,arguments_json TEXT,status TEXT,updated_at INTEGER);`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		_, err = writer.Exec(`INSERT INTO tool_invocation VALUES(?,'own-run','builtin__computer_invoke','{"method":"getScreenshot","args":["PRIVATE_ARGUMENT"]}','succeeded',?)`, fmt.Sprintf("operation-%02d", i), i)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = writer.Exec(`INSERT INTO tool_invocation VALUES('foreign','foreign-run','builtin__computer_invoke','{"method":"PRIVATE_FOREIGN"}','succeeded',100)`); err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Exec(`UPDATE tool_invocation SET arguments_json='{"method":"PRIVATE_METHOD","args":["PRIVATE_ARGUMENT"]}' WHERE invocation_id='operation-23'`); err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Exec(`INSERT INTO tool_invocation VALUES('discover','own-run','builtin__computer_discover','{}','running',99)`); err != nil {
		t.Fatal(err)
	}
	reader, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	result, err := reader.ComputerActivity(context.Background())
	if err != nil || len(result) != 20 || result[0].ID != "discover" || result[0].Action != "getState" {
		t.Fatal(result, err)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "PRIVATE_") {
		t.Fatal("Computer progress exposed arguments or another agent")
	}
}

func TestWorkspaceWorkersUsesExecutionOrderAndConfiguredAgent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workers.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE agent_config(singleton INTEGER,agent_id TEXT);INSERT INTO agent_config VALUES(1,'owner');CREATE TABLE task(task_id TEXT,task_ref TEXT,title TEXT);INSERT INTO task VALUES('t','task:one','Visible title');CREATE TABLE run(run_id TEXT,agent_id TEXT,task_id TEXT,kind TEXT,worker_position TEXT,status TEXT,created_at INTEGER,started_at INTEGER,ended_at INTEGER);`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 55; i++ {
		_, err = db.Exec(`INSERT INTO run VALUES(?,'owner','t','worker','child','completed',?,?,?)`, fmt.Sprintf("w-%02d", i), 100+i, 200+i, 300+i)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = db.Exec(`INSERT INTO run VALUES('old-created-new-start','owner','t','worker','root','running',1,900,NULL),('queued','owner','t','worker','child','queued',850,NULL,NULL),('foreign','other','t','worker','root','running',999,999,NULL),('main','owner',NULL,'main',NULL,'running',999,999,NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	all, err := reader.WorkspaceWorkers(context.Background(), false)
	if err != nil || len(all.Items) != 50 || !all.More || all.Items[0].ID != "old-created-new-start" || all.Items[1].ID != "queued" || all.Items[2].ID != "w-54" {
		t.Fatal(all, err)
	}
	if all.Items[0].Status != "running" || all.Items[1].Status != "queued" || all.Items[2].Status != "completed" {
		t.Fatal("invented status", all)
	}
	active, err := reader.WorkspaceWorkers(context.Background(), true)
	if err != nil || len(active.Items) != 1 || active.Items[0].ID != "old-created-new-start" || active.More {
		t.Fatal(active, err)
	}
}
