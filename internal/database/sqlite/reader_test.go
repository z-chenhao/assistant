package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/database"
)

func metadataFixture(t *testing.T) (*Reader, *sql.DB, *sql.DB, *sql.DB) {
	t.Helper()
	root := t.TempDir()
	open := func(name, schema string) *sql.DB {
		t.Helper()
		db, err := sql.Open("sqlite", filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		if _, err = db.Exec(schema); err != nil {
			t.Fatal(err)
		}
		return db
	}
	state := open("state.db", `CREATE TABLE agent_config(singleton INTEGER,agent_id TEXT); INSERT INTO agent_config VALUES(1,'owner');
 CREATE TABLE task(task_id TEXT,task_ref TEXT,agent_id TEXT); INSERT INTO task VALUES('task-local','task:local','owner'),('task-foreign','task:foreign','foreign');
 CREATE TABLE run(run_id TEXT,agent_id TEXT,created_at INTEGER,kind TEXT,worker_position TEXT,status TEXT,depth INTEGER,task_epoch INTEGER,version INTEGER,started_at INTEGER,ended_at INTEGER,task_id TEXT,parent_run_id TEXT,root_run_id TEXT,checkpoint_json TEXT);
 CREATE TABLE main_input(message_id TEXT,run_id TEXT); CREATE TABLE main_reply(message_id TEXT,run_id TEXT);
 CREATE TABLE tool_invocation(invocation_id TEXT,run_id TEXT,tool_name TEXT);
 INSERT INTO tool_invocation VALUES('remember','root','builtin__memory_manage'),('foreign-inv','foreign','builtin__memory_manage'),('echo','root','builtin__echo');
 CREATE TABLE artifact(artifact_ref TEXT,created_at INTEGER,name TEXT,media_type TEXT,byte_size INTEGER,content_hash TEXT,visibility TEXT,version INTEGER,expires_at INTEGER,deleted_at INTEGER,task_id TEXT,run_id TEXT,owner_user_id TEXT,owner_agent_id TEXT,storage_key TEXT,put_key TEXT);
 CREATE TABLE artifact_reference(artifact_ref TEXT,holder_kind TEXT,holder_id TEXT);
 INSERT INTO artifact_reference VALUES('artifact:local','conversation','m1'),('artifact:local','conversation','foreign'),('artifact:local','run','root'),('artifact:local','task','task-local'),('artifact:local','tool','remember'),('artifact:local','tool','foreign-inv');
 INSERT INTO artifact VALUES('artifact:local',1000,'<script>private()</script>','text/plain',12,'visible-hash','run',1,9000,NULL,'task-local','root','local','owner','DO_NOT_SHOW_STORAGE','DO_NOT_SHOW_PUT_KEY');
 INSERT INTO artifact VALUES('artifact:foreign',1000,'FOREIGN_FILE','text/plain',12,'hidden-hash','run',1,9000,NULL,'task-foreign','foreign','local','foreign','DO_NOT_SHOW_STORAGE','DO_NOT_SHOW_PUT_KEY');
 INSERT INTO artifact VALUES('artifact:other-user',1000,'OTHER_USER_FILE','text/plain',12,'hidden-hash','run',1,9000,NULL,NULL,NULL,'other','owner','DO_NOT_SHOW_STORAGE','DO_NOT_SHOW_PUT_KEY');
 INSERT INTO main_input VALUES('m1','root'); INSERT INTO main_reply VALUES('m2','child');`)
	history := open("conversation.db", `CREATE TABLE conversation_scope(conversation_id TEXT,user_id TEXT,agent_id TEXT);
 INSERT INTO conversation_scope VALUES('owner','local','owner'),('foreign','local','foreign'),('other-user','other','owner');
 CREATE TABLE conversation_message(message_id TEXT,created_at INTEGER,sequence INTEGER,role TEXT,source TEXT,content_json TEXT,conversation_id TEXT);
 INSERT INTO conversation_message VALUES('m1',1000,1,'user','cli','[{"type":"text","text":"DO_NOT_SHOW_MESSAGE"},{"type":"file","artifact_ref":"artifact:local"},{"type":"file","artifact_ref":"artifact:foreign"}]','owner');
 INSERT INTO conversation_message VALUES('m2',1000,2,'assistant','main','[{"type":"text","text":"DO_NOT_SHOW_REPLY"}]','owner');
 INSERT INTO conversation_message VALUES('foreign',1000,3,'user','cli','[{"type":"text","text":"FOREIGN_MESSAGE"}]','foreign');
 INSERT INTO conversation_message VALUES('other-user',1000,4,'user','cli','[{"type":"text","text":"OTHER_USER_MESSAGE"}]','other-user');`)
	memory := open("memory.db", `CREATE TABLE memory_record(memory_id TEXT,user_id TEXT,scope_type TEXT,scope_id TEXT,tier TEXT,content TEXT,source_refs TEXT,supersedes_memory_id TEXT,created_at INTEGER,deleted_at INTEGER,version INTEGER);
 INSERT INTO memory_record VALUES('legacy','local','agent','owner','core','DO_NOT_SHOW_LEGACY_FACT','[]',NULL,1000,NULL,1);
 INSERT INTO memory_record VALUES('sealed','local','agent','owner','recall','sealed:v1:DO_NOT_SHOW_CIPHERTEXT','[{"source_type":"conversation","source_ref":"conversation:m1"},{"source_type":"conversation","source_ref":"conversation:foreign"},{"source_type":"task","source_ref":"task:local"},{"source_type":"task","source_ref":"task:foreign"}]','legacy',2000,NULL,1);
 INSERT INTO memory_record VALUES('deleted','local','user','local','core','DO_NOT_SHOW_DELETED','[]',NULL,3000,4000,2);
 INSERT INTO memory_record VALUES('feedback','local','agent','owner','core','sealed:v1:DO_NOT_SHOW_FEEDBACK','[{"source_type":"feedback","source_ref":"feedback:remember"},{"source_type":"feedback","source_ref":"feedback:foreign-inv"},{"source_type":"feedback","source_ref":"feedback:echo"}]',NULL,2500,NULL,1);
 INSERT INTO memory_record VALUES('foreign','local','agent','foreign','core','FOREIGN_MEMORY','[]',NULL,1000,NULL,1);
 INSERT INTO memory_record VALUES('other-user','other','agent','owner','core','OTHER_USER_MEMORY','[]',NULL,1000,NULL,1);`)
	for i, id := range []string{"root", "child", "third", "fourth", "fifth", "foreign"} {
		owner, parent, position := "owner", "", "root"
		if id == "foreign" {
			owner = "foreign"
		}
		if id == "child" {
			parent, position = "root", "child"
		}
		_, err := state.Exec(`INSERT INTO run VALUES(?,?,1000,'worker',?,'completed',?,1,2,1000,2000,'task-local',?,'root','{"private":"DO_NOT_SHOW_CHECKPOINT"}')`, id, owner, position, i, parent)
		if err != nil {
			t.Fatal(err)
		}
	}
	r, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r, state, history, memory
}

func TestMetadataProjectionScopesRelationsAndNeverReturnsValues(t *testing.T) {
	r, _, _, _ := metadataFixture(t)
	ctx := context.Background()
	counts := map[string]int{"executions": 5, "messages": 2, "memories": 4, "artifacts": 1}
	for kind, count := range counts {
		page, err := r.Browse(ctx, database.Query{Collection: kind})
		if err != nil || len(page.Items) != count {
			t.Fatal("projection count", kind, len(page.Items), err)
		}
		raw, _ := json.Marshal(page)
		for _, forbidden := range []string{"DO_NOT_SHOW", "FOREIGN_", "OTHER_USER_", "storage_key", "put_key", "checkpoint_json", "content_json", ":foreign", ":other-user"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatal("private metadata projection escaped its allowlist", kind, forbidden)
			}
		}
	}
	checks := []struct{ kind, ref, relation, target string }{
		{"executions", "run:child", "Parent execution", "run:root"},
		{"executions", "run:child", "Task", "task:local"},
		{"messages", "conversation:m1", "Attachment", "artifact:local"},
		{"messages", "conversation:m1", "Execution", "run:root"},
		{"messages", "conversation:m2", "Execution", "run:child"},
		{"memories", "memory:sealed", "Replaces", "memory:legacy"},
		{"memories", "memory:legacy", "Replaced by", "memory:sealed"},
		{"memories", "memory:sealed", "Source message", "conversation:m1"},
		{"memories", "memory:sealed", "Source task", "task:local"},
		{"memories", "memory:feedback", "Source execution", "run:root"},
		{"artifacts", "artifact:local", "Produced by", "run:root"},
		{"artifacts", "artifact:local", "Attached to message", "conversation:m1"},
		{"artifacts", "artifact:local", "Used by task", "task:local"},
	}
	for _, check := range checks {
		page, err := r.Browse(ctx, database.Query{Collection: check.kind, Ref: check.ref})
		if err != nil || len(page.Items) != 1 {
			t.Fatal("exact projection", check.ref, err)
		}
		found := false
		for _, link := range page.Items[0].Links {
			found = found || link.Relation == check.relation && link.Ref == check.target
		}
		if !found {
			t.Fatal("missing causal relationship", check)
		}
	}
	for _, ref := range []string{"run:foreign", "conversation:foreign", "conversation:other-user", "memory:foreign", "memory:other-user", "artifact:foreign", "artifact:other-user", "run:missing"} {
		if _, err := r.Browse(ctx, database.Query{Collection: database.Collection(ref), Ref: ref}); err != database.ErrNotFound {
			t.Fatal("unknown and inaccessible references differ", ref, err)
		}
	}
}

func TestMetadataKeysetPaginationAndReadOnlyPools(t *testing.T) {
	r, state, history, memory := metadataFixture(t)
	ctx := context.Background()
	q := database.Query{Collection: "executions", Limit: 2}
	seen := map[string]bool{}
	for page := 0; page < 3; page++ {
		result, err := r.Browse(ctx, q)
		if err != nil || len(result.Items) > 2 {
			t.Fatal("bounded continuation failed", err)
		}
		for _, item := range result.Items {
			if seen[item.Ref] {
				t.Fatal("duplicate record on equal-time continuation")
			}
			seen[item.Ref] = true
		}
		q.Cursor = result.Next
	}
	if len(seen) != 5 || q.Cursor != "" {
		t.Fatal("pagination lost records", len(seen))
	}
	for name, db := range map[string]*sql.DB{"state": r.state, "history": r.history, "memory": r.memory} {
		if _, err := db.ExecContext(ctx, `CREATE TABLE unauthorized_write(value TEXT)`); err == nil {
			t.Fatal("metadata pool accepted a write", name)
		}
	}
	for _, db := range []*sql.DB{state, history, memory} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='unauthorized_write'`).Scan(&n); err != nil || n != 0 {
			t.Fatal("inspection changed a domain database", n, err)
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := r.Browse(ctx, database.Query{Collection: "executions"}); err != database.ErrUnavailable {
		t.Fatal("cancelled inspection did not fail closed", err)
	}
}

func TestMissingDatabasesAreNotCreated(t *testing.T) {
	root := t.TempDir()
	r, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, err := r.Browse(context.Background(), database.Query{Collection: "executions"}); err != database.ErrUnavailable {
		t.Fatal("missing data appeared successful", err)
	}
	for _, name := range []string{"state.db", "conversation.db", "memory.db"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatal("inspection created a domain database", name, err)
		}
	}
}
