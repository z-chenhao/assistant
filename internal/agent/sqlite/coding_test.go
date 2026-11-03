package sqlite

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/computer/environment"
	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/task"
	"github.com/z-chenhao/assistant/internal/tool"
)

func codeFixture(t *testing.T) (*Store, *Runs, agent.Actor, *loopProvider, environment.Config) {
	t.Helper()
	s, r, a, p := taskFixture(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "file.txt"), []byte("dirty source"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := environment.Config{Root: filepath.Join(root, "execution"), RepositorySource: source}
	echo, _ := tool.FixtureRegistry().Resolve("mcp__fixture__echo")
	defs := append(task.Definitions(), echo)
	defs = append(defs, environment.Definitions(environment.ResourceHash(cfg))...)
	a.Registry, err = tool.NewRegistry(defs)
	if err != nil {
		t.Fatal(err)
	}
	r = s.Runs(r.history, a.Registry)
	r.ConfigureLocalEnvironment(cfg)
	a.Store = r
	return s, r, a, p, cfg
}

func TestTaskCodePreparationAndFailureStayScoped(t *testing.T) {
	s, _, a, p, cfg := codeFixture(t)
	ctx := context.Background()
	p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
		if isRoot(req) || isFollowup(req) || req.Messages[len(req.Messages)-1].Role == "tool" {
			return llm.GenerationResult{Text: "observed"}, nil
		}
		return calls(task.ManageName, `{"operation":"create","task":"prepare code"}`), nil
	}
	mustSubmit(t, a, "one", "first task")
	mustDrain(t, a)
	var ref string
	if err := s.db.QueryRow(`SELECT task_ref FROM task`).Scan(&ref); err != nil {
		t.Fatal(err)
	}
	w, err := s.CodeReport(ctx, ref, false)
	if err != nil || w.Status != "ready" {
		t.Fatal(w, err)
	}
	// New source failure affects only the next Task's Root; the existing copy
	// and original Task remain available and the Actor still drains follow-ups.
	if err := os.Remove(filepath.Join(cfg.RepositorySource, ".git")); err != nil {
		t.Fatal(err)
	}
	mustSubmit(t, a, "two", "second task")
	mustDrain(t, a)
	if scalar(t, s, `SELECT count(*) FROM task_code_environment WHERE status='ready'`) != 1 || scalar(t, s, `SELECT count(*) FROM task_code_environment WHERE status='failed'`) != 1 || scalar(t, s, `SELECT count(*) FROM run WHERE kind='worker' AND status='failed'`) != 1 {
		t.Fatal("failed preparation escaped Task boundary")
	}
	other, err := Open(ctx, filepath.Dir(s.path))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	again, err := other.CodeReport(ctx, ref, false)
	if err != nil || again.Directory != w.Directory || again.BaselineHash != w.BaselineHash {
		t.Fatal("restart lost prepared directory", again, err)
	}
}

func TestTaskNativeDispatchUsesItsFrozenDirectory(t *testing.T) {
	s, r, a, p, cfg := codeFixture(t)
	// Only the external execution peer is replaced. Task create, frozen tools,
	// approvals, invocation dispatch and directory preparation are production.
	seen := ""
	r.localExecute = func(_ context.Context, got environment.Config, name, args string) tool.Result {
		seen = got.Root
		if name != "builtin__write" || got.Root == cfg.Root {
			return tool.Failure("PERMISSION_DENIED")
		}
		var input struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if json.Unmarshal([]byte(args), &input) != nil {
			return tool.Failure("INVALID_ARGUMENT")
		}
		if err := os.WriteFile(filepath.Join(got.Root, input.Path), []byte(input.Content), 0600); err != nil {
			return tool.Failure("PROVIDER_UNAVAILABLE")
		}
		return tool.Result{Text: `{"bytes":8}`}
	}
	p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
		last := req.Messages[len(req.Messages)-1]
		if isRoot(req) {
			if last.Role == "tool" {
				return llm.GenerationResult{Text: "done"}, nil
			}
			return calls("builtin__write", `{"path":"result.txt","content":"approved"}`), nil
		}
		if isFollowup(req) || last.Role == "tool" {
			return llm.GenerationResult{Text: "accepted"}, nil
		}
		return calls(task.ManageName, `{"operation":"create","task":"code work"}`), nil
	}
	mustSubmit(t, a, "write", "start code task")
	mustDrain(t, a)
	if seen != "" {
		t.Fatal("write dispatched before approval")
	}
	approvePolicy(t, s, r)
	mustDrain(t, a)
	if seen == "" || !strings.HasSuffix(seen, string(filepath.Separator)+"work") {
		t.Fatal("scoped directory not supplied", seen)
	}
	if _, err := os.Stat(filepath.Join(cfg.Root, "result.txt")); !os.IsNotExist(err) {
		t.Fatal("shared root polluted")
	}
	raw, err := os.ReadFile(filepath.Join(seen, "result.txt"))
	if err != nil || string(raw) != "approved" {
		t.Fatal(string(raw), err)
	}
	if scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE tool_name='builtin__write' AND status='succeeded' AND attempt_count=1`) != 1 {
		t.Fatal("approved action not committed once")
	}
}
