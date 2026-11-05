package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/artifact"
	artifacttools "github.com/z-chenhao/assistant/internal/artifact/tools"
	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/tool"
	"github.com/z-chenhao/assistant/internal/tool/codemode"
	"github.com/z-chenhao/assistant/internal/webaccess"
)

func TestWebSearchDurableGenerationBudgetAndFences(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "persisted and never redispatched", true: "budget prevents dispatch"}[blocked], func(t *testing.T) {
			ctx := context.Background()
			calls := []llm.ToolCall{{ID: "search", Name: webaccess.SearchName, Arguments: `{"query":"public query"}`}}
			s, r, c, executor, exchange := admitWebSearch(t, calls, blocked)
			defer r.Release(ctx, c)
			invs, e := executor.Store.Accept(ctx, exchange, calls, executor.Registry.Describe())
			if e != nil {
				t.Fatal(e)
			}
			if e = executor.Store.Start(ctx, invs[0].ID); e != nil {
				t.Fatal(e)
			}
			run, e := r.Load(ctx, c)
			if e != nil {
				t.Fatal(e)
			}
			dispatched := 0
			p := &loopProvider{store: s, respond: func(req llm.GenerationRequest) (llm.GenerationResult, error) {
				dispatched++
				before, _ := encoded(run.Snapshot)
				after, _ := encoded(req.Snapshot)
				if before != after || len(req.Messages) != 2 || req.Messages[1].Text != "public query" || req.WebSearch.InvocationID != invs[0].ID || len(req.Tools) != 0 {
					t.Fatal(req)
				}
				for _, m := range req.Messages {
					if strings.Contains(m.Text, "do the requested work") {
						t.Fatal("history leaked")
					}
				}
				return llm.GenerationResult{Text: "evidence", WebSearch: &llm.WebSearchResult{Sources: []llm.WebSource{{URL: "https://example.com/"}}}, Usage: &llm.TokenUsage{InputTokens: 7, OutputTokens: 2}}, nil
			}}
			r.ConfigureWebAccess(webaccess.Config{}, p)
			ts := executor.Store.(*tools)
			first, e := ts.hostedSearch(ctx, invs[0], run, "public query")
			if blocked {
				if e == nil || e.Error() != "BUDGET_APPROVAL_REQUIRED" || dispatched != 0 {
					t.Fatal(first, e, dispatched)
				}
				return
			}
			if e != nil || first.WebSearch == nil || dispatched != 1 {
				t.Fatal(first, e, dispatched)
			}
			again, e := ts.hostedSearch(ctx, invs[0], run, "public query")
			if e != nil || again.Text != first.Text || dispatched != 1 {
				t.Fatal(again, e, dispatched)
			}
			if e = r.write(ctx, func(tx *sql.Tx) error {
				var taskID string
				if e := tx.QueryRowContext(ctx, `SELECT task_id FROM run WHERE run_id=?`, run.ID).Scan(&taskID); e != nil {
					return e
				}
				messages, e := r.taskHandoff(ctx, tx, taskID, 2)
				if e == nil && len(messages) != 0 {
					t.Fatal("search evidence became final task handoff", messages)
				}
				return e
			}); e != nil {
				t.Fatal(e)
			}
			if scalar(t, s, `SELECT count(*) FROM model_generation WHERE run_id=? AND step_index<0 AND status='completed'`, run.ID) != 1 || scalar(t, s, `SELECT count(*) FROM provider_usage WHERE generation_id IN (SELECT generation_id FROM model_generation WHERE run_id=? AND step_index<0)`, run.ID) != 1 {
				t.Fatal("exchange or usage missing")
			}
			if e = r.Release(ctx, c); e != nil {
				t.Fatal(e)
			}
			if _, e = ts.hostedSearch(ctx, invs[0], run, "public query"); e == nil {
				t.Fatal("expired claim accepted")
			}
		})
	}
}

func TestWebAccessUsesFrozenPolicyAndAcceptedArguments(t *testing.T) {
	ctx := context.Background()
	calls := []llm.ToolCall{{ID: "fetch", Name: webaccess.FetchName, Arguments: `{"url":"http://127.0.0.1/private"}`}}
	_, r, c, executor, exchange := admitPolicy(t, webaccess.Definitions(webaccess.Config{}), calls)
	defer r.Release(ctx, c)
	invs, e := executor.Store.Accept(ctx, exchange, calls, executor.Registry.Describe())
	if e != nil {
		t.Fatal(e)
	}
	if e = executor.Store.Start(ctx, invs[0].ID); e != nil {
		t.Fatal(e)
	}
	forged := invs[0]
	forged.Call.Arguments = `{"url":"https://example.com/"}`
	result, e := executor.Store.(tool.HostStore).ExecuteBuiltin(ctx, forged)
	if e != nil || result.Error == nil || result.Error.Code != "WEB_ADDRESS_BLOCKED" {
		t.Fatal(result, e)
	}
	r.ConfigureWebAccess(webaccess.Config{ExaFallback: true}, nil)
	result, e = executor.Store.(tool.HostStore).ExecuteBuiltin(ctx, invs[0])
	raw, _ := json.Marshal(result)
	if e != nil || result.Error == nil || result.Error.Code != "MODEL_INCOMPATIBLE" {
		t.Fatal(string(raw), e)
	}
}

func admitWebSearch(t *testing.T, calls []llm.ToolCall, priced bool) (*Store, *Runs, agent.RunClaim, tool.Executor, string) {
	t.Helper()
	ctx := context.Background()
	s, r, _, a, p := loopFixture(t)
	registry, e := tool.NewRegistry(append(webaccess.Definitions(webaccess.Config{}), agent.AgentToolDefinition()))
	if e != nil {
		t.Fatal(e)
	}
	r.registry, a.Registry = registry, registry
	if priced {
		r.ConfigureSpend(spendPolicy(t, 15000))
	}
	p.respond = func(llm.GenerationRequest) (llm.GenerationResult, error) {
		return llm.GenerationResult{ToolCalls: calls}, nil
	}
	runID := admitToolRun(t, s, r, a, registry.Describe(), nil)
	c, e := r.Claim(ctx, runID)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Initialize(ctx, c); e != nil {
		t.Fatal(e)
	}
	run, e := r.Load(ctx, c)
	if e != nil {
		t.Fatal(e)
	}
	generation, e := r.Generation(ctx, c, run)
	if e != nil || generation.Waiting {
		t.Fatal(generation, e)
	}
	if _, e = p.Generate(ctx, generation.Request, nil); e != nil {
		t.Fatal(e)
	}
	return s, r, c, tool.Executor{Registry: registry, Store: r.ToolStore(c), Concurrency: 1}, generation.Request.GenerationID
}

func TestWebAccessPageArtifactContinuationAndReplay(t *testing.T) {
	ctx := context.Background()
	calls := []llm.ToolCall{{ID: "fetch", Name: webaccess.FetchName, Arguments: `{"url":"https://example.com/article"}`}}
	defs := append(webaccess.Definitions(webaccess.Config{}), artifacttools.Definitions()...)
	s, r, c, e, exchange := admitPolicy(t, defs, calls)
	defer r.Release(ctx, c)
	e.Concurrency = 1
	full := strings.Repeat("事实 ", 1200) + "unique continuation marker"
	fetched := 0
	r.webFetch = func(_ context.Context, raw string) (webaccess.Page, error) {
		fetched++
		if raw != "https://example.com/article" {
			t.Fatal(raw)
		}
		return webaccess.Page{URL: raw, Text: full, Untrusted: true, RetrievedAt: "2026-11-06T00:00:00Z"}, nil
	}
	results, err := e.Execute(ctx, exchange, calls)
	if err != nil || len(results) != 1 || results[0].Error != nil || len(results[0].ArtifactRefs) != 1 {
		t.Fatal(results, err)
	}
	var envelope struct {
		Artifact string `json:"artifact_ref"`
		More     bool   `json:"has_more"`
		Text     string `json:"text"`
		Bytes    int    `json:"total_bytes"`
	}
	if json.Unmarshal([]byte(results[0].Text), &envelope) != nil || !envelope.More || envelope.Bytes != len(full) || len(envelope.Text) > 4096 || envelope.Artifact != results[0].ArtifactRefs[0] {
		t.Fatal(results)
	}
	again, err := e.Execute(ctx, exchange, calls)
	if err != nil || fetched != 1 || len(again) != 1 || again[0].Text != results[0].Text {
		t.Fatal(again, err, fetched)
	}
	var scope artifact.Scope
	if err = s.Transaction(ctx, func(tx *sql.Tx) error { var e error; scope, e = runScope(ctx, tx, c.RunID()); return e }); err != nil {
		t.Fatal(err)
	}
	// Read the persisted bytes under the same scope used by artifact_manage.
	reader, err := s.Artifacts().Open(ctx, scope, envelope.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil || string(data) != full {
		t.Fatal("persisted continuation differs", err)
	}
}

func TestWebAccessMainUsesRealCodeMode(t *testing.T) {
	defs := append(webaccess.Definitions(webaccess.Config{}), codemode.Definition())
	s, r, a, p := childFixture(t, defs...)
	r.ConfigureJavaScript(realCodeRuntime(t))
	r.webFetch = func(_ context.Context, raw string) (webaccess.Page, error) {
		return webaccess.Page{URL: raw, Text: "public page evidence", Untrusted: true}, nil
	}
	p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
		if req.Messages[len(req.Messages)-1].Role == "tool" {
			return llm.GenerationResult{Text: "read completed"}, nil
		}
		args, _ := json.Marshal(map[string]string{"source": `return await tools.builtin__web_fetch({url:"https://example.com/"});`})
		return calls(codemode.ToolName, string(args)), nil
	}
	mustSubmit(t, a, "web-access-main", "Read the public page")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := a.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if scalar(t, s, `SELECT count(*) FROM tool_invocation i JOIN run r USING(run_id) WHERE r.kind='main' AND i.tool_name=? AND i.status='succeeded' AND i.parent_invocation_id IS NOT NULL`, webaccess.FetchName) != 1 {
		t.Fatal("Main cannot use connector through Code Mode")
	}
	if scalar(t, s, `SELECT count(*) FROM artifact`) != 1 {
		t.Fatal("no persisted page")
	}
}
