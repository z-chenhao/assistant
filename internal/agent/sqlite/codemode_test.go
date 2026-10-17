package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/tool"
	"github.com/z-chenhao/erii/internal/tool/codemode"
)

func realCodeRuntime(t *testing.T) *codemode.Sidecar {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node is required for Sidecar contract tests")
	}
	script, err := filepath.Abs("../../../runtime/javascript/src/sidecar.mjs")
	if err != nil {
		t.Fatal(err)
	}
	return &codemode.Sidecar{Command: node, Script: script}
}
func TestCodeModeRealRuntimeNestedToolAndStore(t *testing.T) {
	s, r, a, p := childFixture(t, codemode.Definition())
	r.ConfigureJavaScript(realCodeRuntime(t))
	p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
		if req.Messages[len(req.Messages)-1].Role == "tool" {
			return llm.GenerationResult{Text: "observed"}, nil
		}
		source := `store("persisted", {n: 5}); const x = await tools.builtin__text__echo({text:"nested"}); return [x, load("persisted")];`
		args, _ := json.Marshal(map[string]string{"source": source})
		return calls(codemode.ToolName, string(args)), nil
	}
	mustSubmit(t, a, "code", "execute")
	mustDrain(t, a)
	for query, want := range map[string]int{
		`SELECT count(*) FROM code_execution`:                                                             1,
		`SELECT count(*) FROM code_call`:                                                                  1,
		`SELECT count(*) FROM code_store WHERE json_extract(value_json,'$.n')=5`:                          1,
		`SELECT count(*) FROM tool_invocation WHERE status='succeeded'`:                                   2,
		`SELECT count(*) FROM tool_invocation WHERE parent_invocation_id IS NOT NULL`:                     1,
		`SELECT count(*) FROM code_event WHERE json_extract(message_json,'$.kind')='execution_completed'`: 1,
	} {
		if got := scalar(t, s, query); got != want {
			t.Fatalf("%s = %d, want %d", query, got, want)
		}
	}
	var result string
	if err := s.db.QueryRow(`SELECT result_json FROM tool_invocation WHERE tool_name=?`, codemode.ToolName).Scan(&result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, `nested`) || !strings.Contains(result, `return`) {
		t.Fatal(result)
	}
}
func TestCodeModeMigrateEmptyAndV15(t *testing.T) {
	for _, prior := range []int{0, 15} {
		t.Run(strconv.Itoa(prior), func(t *testing.T) {
			ctx := context.Background()
			db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if prior > 0 {
				if err = migrate(ctx, db, migrations[:prior]); err != nil {
					t.Fatal(err)
				}
			}
			broken := append([]string{}, migrations...)
			broken[len(broken)-1] += "\nINVALID SQL;"
			if err = migrate(ctx, db, broken); err == nil {
				t.Fatal("accepted broken migration")
			}
			var version int
			db.QueryRow(`PRAGMA user_version`).Scan(&version)
			want := prior
			if version != want {
				t.Fatalf("failed migration version %d", version)
			}
			if err = migrate(ctx, db, migrations); err != nil {
				t.Fatal(err)
			}
			if err = migrate(ctx, db, migrations); err != nil {
				t.Fatal(err)
			}
			var tables int
			db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('code_execution','code_call','code_store','code_event')`).Scan(&tables)
			if tables != 4 {
				t.Fatal(tables)
			}
		})
	}
}
func TestNestedAdmissionRejectsUnacceptedParent(t *testing.T) {
	_, r, a, _ := childFixture(t, codemode.Definition())
	receipt := mustSubmit(t, a, "parent-guard", "hello")
	claim, err := r.Claim(context.Background(), receipt.RunID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (nestedTools{&tools{r, claim}, "forged-parent"}).Accept(context.Background(), "forged-exchange", []llm.ToolCall{{ID: "1", Name: "builtin__text__echo", Arguments: `{"text":"x"}`}}, a.Registry.Describe())
	if err == nil {
		t.Fatal("accepted forged parent")
	}
}

var _ tool.Store = nestedTools{}

func TestCodeModeStoreAndEventCommitFailuresDoNotLeakSuccess(t *testing.T) {
	for _, boundary := range []string{"store", "terminal-event"} {
		t.Run(boundary, func(t *testing.T) {
			s, r, a, p := childFixture(t, codemode.Definition())
			r.ConfigureJavaScript(realCodeRuntime(t))
			trigger := `CREATE TRIGGER reject_store BEFORE INSERT ON code_store BEGIN SELECT RAISE(ABORT,'injected store failure'); END`
			source := `store("x",1); return "UNCOMMITTED_SUCCESS";`
			if boundary == "terminal-event" {
				trigger = `CREATE TRIGGER reject_code_event BEFORE INSERT ON code_event WHEN json_extract(NEW.message_json,'$.kind')='execution_completed' BEGIN SELECT RAISE(ABORT,'injected event failure'); END`
				source = `return "UNCOMMITTED_SUCCESS";`
			}
			if _, err := s.db.Exec(trigger); err != nil {
				t.Fatal(err)
			}
			p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
				if req.Messages[len(req.Messages)-1].Role == "tool" {
					return llm.GenerationResult{Text: "observed error"}, nil
				}
				args, _ := json.Marshal(map[string]string{"source": source})
				return calls(codemode.ToolName, string(args)), nil
			}
			mustSubmit(t, a, boundary, "execute")
			mustDrain(t, a)
			if scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE tool_name='builtin__codemode__execute' AND status='failed'`) != 1 {
				t.Fatal("reported success before commit")
			}
			if scalar(t, s, `SELECT count(*) FROM code_store`) != 0 {
				t.Fatal("failed Store persisted")
			}
			var raw string
			if err := s.db.QueryRow(`SELECT result_json FROM tool_invocation WHERE tool_name=?`, codemode.ToolName).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(raw, "UNCOMMITTED_SUCCESS") || strings.Contains(raw, "injected") {
				t.Fatal("uncommitted data or internal error exposed", raw)
			}
		})
	}
}
