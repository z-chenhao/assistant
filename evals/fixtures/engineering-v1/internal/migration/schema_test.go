package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationUpDownAndRepeat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	initial := []byte(`{"worker_job":[{"id":"root","task_id":"one","parent_run_id":"","epoch":1}]}`)
	if err := os.WriteFile(path, initial, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Up(path); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]json.RawMessage
	if json.Unmarshal(after, &state) != nil || state["worker_job"] != nil || state["run_tree"] == nil {
		t.Fatalf("not migrated: %s", after)
	}
	if err := Up(path); err != nil {
		t.Fatal(err)
	}
	same, _ := os.ReadFile(path)
	if string(same) != string(after) {
		t.Fatal("migration repeated")
	}
	if err := Down(path); err != nil {
		t.Fatal(err)
	}
	back, _ := os.ReadFile(path)
	if json.Unmarshal(back, &state) != nil {
		t.Fatal("invalid down")
	}
	var down map[string]json.RawMessage
	_ = json.Unmarshal(back, &down)
	if down["worker_job"] == nil || down["run_tree"] != nil {
		t.Fatalf("down: %s", back)
	}
}

func TestMigrationEmptyAndInvalidPreservesInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Up(path); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	var state map[string]json.RawMessage
	_ = json.Unmarshal(b, &state)
	if state["run_tree"] == nil || state["worker_job"] != nil {
		t.Fatalf("empty upgrade: %s", b)
	}
	if err := os.WriteFile(path, []byte("bad json"), 0600); err != nil {
		t.Fatal(err)
	}
	if Up(path) == nil {
		t.Fatal("invalid input accepted")
	}
	b, _ = os.ReadFile(path)
	if string(b) != "bad json" {
		t.Fatal("invalid input overwritten")
	}
}
