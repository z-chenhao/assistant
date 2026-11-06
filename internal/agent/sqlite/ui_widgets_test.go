package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/conversation"
	"github.com/z-chenhao/assistant/internal/delivery/ui"
	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/tool"
	"github.com/z-chenhao/assistant/internal/tool/codemode"
)

func TestMainUIWidgetUsesCodeModeAndDurableReply(t *testing.T) {
	s, r, a, p := childFixture(t, ui.ShowDefinition(), codemode.Definition())
	r.ConfigureJavaScript(realCodeRuntime(t))
	var previewed atomic.Bool
	var committed atomic.Bool
	a.Sink = func(e agent.Event) {
		if e.Kind == "ui_preview_update" && e.UIPreview != nil && len(e.UIPreview.Components) > 0 {
			previewed.Store(true)
		}
		if e.Kind == "ui_preview_committed" {
			committed.Store(true)
		}
	}
	defs := []tool.Definition{}
	for _, spec := range r.registry.Describe() {
		d, _ := r.registry.Resolve(spec.Name)
		d.Spec.CodeModeOnly = spec.Name != codemode.ToolName
		defs = append(defs, d)
	}
	registry, err := tool.NewRegistry(defs)
	if err != nil {
		t.Fatal(err)
	}
	r.registry = registry
	a.Registry = registry
	for _, spec := range agent.WorkerTools(registry.Describe()) {
		if spec.Name == ui.ShowName {
			t.Fatal("Worker can present UI")
		}
	}
	p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
		if len(req.Tools) != 1 || req.Tools[0].Name != codemode.ToolName {
			t.Fatal("UI became a native model tool", req.Tools)
		}
		count := 0
		for _, m := range req.Messages {
			if m.Role == "system" {
				count++
			}
		}
		if count != 1 {
			t.Fatal("Agent used split system messages")
		}
		if req.Messages[len(req.Messages)-1].Role == "tool" {
			if !previewed.Load() {
				t.Error("Code Mode UI was withheld until final reply")
			}
			return llm.GenerationResult{Text: "Explore the comparison."}, nil
		}
		doc, _ := json.Marshal(ui.Widget{Title: "Comparison", Fallback: "The result is 42.", Components: []ui.Component{{Name: "Table", Props: json.RawMessage(`{"title":"Result","columns":["Value"],"rows":[["42"]],"source":"Calculated fixture"}`)}}})
		args, _ := json.Marshal(map[string]string{"source": "return await tools.builtin__intelligent_ui_show(" + string(doc) + ");"})
		return calls(codemode.ToolName, string(args)), nil
	}
	mustSubmit(t, a, "ui-widget", "Show the useful answer")
	mustDrain(t, a)
	if !previewed.Load() || !committed.Load() {
		t.Fatal("preview lifecycle missing", previewed.Load(), committed.Load())
	}
	if scalar(t, s, `SELECT count(*) FROM run_trace WHERE json_extract(event_json,'$.kind') LIKE 'ui_preview_%'`) != 0 {
		t.Fatal("preview persisted into execution trace")
	}
	if scalar(t, s, `SELECT count(*) FROM ui_widget WHERE message_id IS NOT NULL`) != 1 || scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE tool_name='builtin__intelligent_ui_show' AND status='succeeded' AND attempt_count=1`) != 1 {
		t.Fatal("widget missing or duplicated")
	}
	var ref, messageID string
	if err = s.db.QueryRow(`SELECT ui_ref,message_id FROM ui_widget`).Scan(&ref, &messageID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WebWidget(context.Background(), ref); err != agent.ErrNotFound {
		t.Fatal("native component exposed as sandbox", err)
	}
	var raw string
	if err = s.db.QueryRow(`SELECT message_json FROM conversation_intent WHERE message_id=?`, messageID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var m conversation.Message
	if json.Unmarshal([]byte(raw), &m) != nil {
		t.Fatal(raw)
	}
	projection, err := s.WebProjection(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	var spec ui.Spec
	if json.Unmarshal(projection, &spec) != nil {
		t.Fatal(string(projection))
	}
	found := false
	for _, c := range spec.Components {
		found = found || c.Type == "openintelligentui" && c.Name == "Table"
	}
	if !found {
		t.Fatal("reply lost UI reference", spec)
	}
	bubbles, err := s.WebBubbles(context.Background(), m, spec)
	if err != nil || len(bubbles) != 2 || len(bubbles[0].Components) != 1 || bubbles[0].Components[0].Name != "Table" || bubbles[1].Fallback != "Explore the comparison." {
		t.Fatal("UI must precede the separate final reply", bubbles, err)
	}
	again, err := s.WebProjection(context.Background(), m)
	if err != nil || string(again) != string(projection) {
		t.Fatal("Web bubble grouping changed the frozen delivery", err)
	}
	if _, err = s.WebWidget(context.Background(), "ui:outside"); err != agent.ErrNotFound {
		t.Fatal("out-of-scope UI accessible", err)
	}
	mustDrain(t, a)
	if scalar(t, s, `SELECT count(*) FROM ui_widget`) != 1 {
		t.Fatal("settled UI repeated")
	}
}

func TestUIStagingRecoversAfterResultCommitFailure(t *testing.T) {
	ctx := context.Background()
	calls := []llm.ToolCall{{ID: "show", Name: ui.ShowName, Arguments: `{"title":"Answer","fallback":"The answer is 42.","components":[{"name":"Table","props":{"title":"Answer","columns":["Value"],"rows":[["42"]],"source":"Calculated"}}]}`}}
	s, runs, owner, executor, exchange := admitPolicy(t, []tool.Definition{ui.ShowDefinition()}, calls)
	if _, err := s.db.Exec(`CREATE TRIGGER ui_result_fault BEFORE UPDATE ON tool_invocation WHEN NEW.tool_name='builtin__intelligent_ui_show' AND NEW.status='succeeded' BEGIN SELECT RAISE(ABORT,'owned result commit fault'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.Execute(ctx, exchange, calls); err == nil {
		t.Fatal("result commit fault was not exercised")
	}
	if scalar(t, s, `SELECT count(*) FROM ui_widget WHERE message_id IS NULL`) != 1 {
		t.Fatal("UI staging did not survive its separate result commit")
	}
	var ref string
	if err := s.db.QueryRow(`SELECT ui_ref FROM ui_widget`).Scan(&ref); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WebWidget(ctx, ref); err != agent.ErrNotFound {
		t.Fatal("unpublished UI was visible", err)
	}
	if _, err := s.db.Exec(`DROP TRIGGER ui_result_fault`); err != nil {
		t.Fatal(err)
	}
	if err := runs.Release(ctx, owner); err != nil {
		t.Fatal(err)
	}
	recovered, err := runs.Claim(ctx, owner.RunID())
	if err != nil {
		t.Fatal(err)
	}
	executor.Store = runs.ToolStore(recovered)
	results, err := executor.Execute(ctx, exchange, calls)
	if err != nil || len(results) != 1 || results[0].Error != nil {
		t.Fatal(results, err)
	}
	staged, stagedErr := runs.StagedUIPreviews(ctx, recovered)
	if stagedErr != nil || len(staged) != 1 || "ui:"+staged[0].ID != ref {
		t.Fatal("recovery did not restore staged preview", staged, stagedErr)
	}
	if _, err := runs.StagedUIPreviews(ctx, owner); err != agent.ErrClaimLost {
		t.Fatal("stale claim read preview", err)
	}
	if scalar(t, s, `SELECT count(*) FROM ui_widget`) != 1 || scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE status='succeeded'`) != 1 {
		t.Fatal("recovery duplicated the presentation")
	}
}

type uiStreamingProvider struct {
	*loopProvider
	raw  string
	fail bool
}

func (p uiStreamingProvider) Generate(ctx context.Context, req llm.GenerationRequest, sink func(llm.GenerationEvent)) (llm.GenerationResult, error) {
	if req.Messages[len(req.Messages)-1].Role != "tool" {
		sink(llm.GenerationEvent{Kind: "ui_delta", ToolCall: &llm.ToolCall{ID: "stream-call", Name: ui.ShowName, Arguments: p.raw[:len(p.raw)-4]}})
		if p.fail {
			return llm.GenerationResult{}, llm.ProviderUnavailable
		}
	}
	return p.loopProvider.Generate(ctx, req, sink)
}
func (p uiStreamingProvider) Continue(ctx context.Context, req llm.GenerationRequest, sink func(llm.GenerationEvent)) (llm.GenerationResult, error) {
	return p.Generate(ctx, req, sink)
}
func TestMainDirectUIPreviewIsEphemeralAndWithdrawnOnFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			s, _, a, p := childFixture(t, ui.ShowDefinition())
			raw := `{"title":"Stream","fallback":"Answer","components":[{"name":"Table","props":{"title":"Stream","columns":["Name"],"rows":[["A"]],"source":"User"}}]}`
			p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
				if req.Messages[len(req.Messages)-1].Role == "tool" {
					return llm.GenerationResult{Text: "Done"}, nil
				}
				return calls(ui.ShowName, raw), nil
			}
			a.Provider = uiStreamingProvider{loopProvider: p, raw: raw, fail: fail}
			var early, discarded, committed atomic.Bool
			a.Sink = func(e agent.Event) {
				if e.Kind == "ui_preview_update" && e.UIPreview != nil && len(e.UIPreview.Components) > 0 && scalar(t, s, `SELECT count(*) FROM ui_widget`) == 0 {
					early.Store(true)
				}
				if e.Kind == "ui_preview_discarded" {
					discarded.Store(true)
				}
				if e.Kind == "ui_preview_committed" {
					committed.Store(true)
				}
			}
			mustSubmit(t, a, "preview", "Show")
			err := a.Drain(context.Background())
			if !early.Load() {
				t.Fatal("direct UI did not preview before execution")
			}
			if fail {
				if err == nil || !discarded.Load() || committed.Load() || scalar(t, s, `SELECT count(*) FROM ui_widget`) != 0 {
					t.Fatal("failed prefix became a delivered UI", err)
				}
			} else if err != nil || !committed.Load() || scalar(t, s, `SELECT count(*) FROM ui_widget WHERE message_id IS NOT NULL`) != 1 {
				t.Fatal("final delivery changed", err)
			}
			if scalar(t, s, `SELECT count(*) FROM run_trace WHERE json_extract(event_json,'$.kind') LIKE 'ui_preview_%'`) != 0 {
				t.Fatal("preview persisted")
			}
		})
	}
}

func TestUIPreviewUsesAcceptedArgumentsRatherThanCallerPayload(t *testing.T) {
	ctx := context.Background()
	calls := []llm.ToolCall{{ID: "show", Name: ui.ShowName, Arguments: `{"title":"Accepted","fallback":"Answer","components":[{"name":"Table","props":{"title":"A","columns":["Name"],"rows":[["A"]],"source":"User"}}]}`}}
	_, _, _, executor, exchange := admitPolicy(t, []tool.Definition{ui.ShowDefinition()}, calls)
	invs, err := executor.Store.Accept(ctx, exchange, calls, executor.Registry.Describe())
	if err != nil {
		t.Fatal(err)
	}
	inv, err := executor.Store.Prepare(ctx, invs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := executor.Store.Start(ctx, inv.ID); err != nil {
		t.Fatal(err)
	}
	inv.Call.Arguments = `{"title":"Forged caller payload"}`
	observed := ""
	ctx = ui.WithPreviewObserver(ctx, func(_ string, raw string) { observed = raw })
	result, err := executor.Store.(tool.HostStore).ExecuteBuiltin(ctx, inv)
	if err != nil || result.Error != nil || observed != calls[0].Arguments {
		t.Fatal("preview bypassed the accepted invocation", result, err, observed)
	}
}

func TestMainUIPreviewKeepsDistinctInvocationsForRepeatedProviderCallID(t *testing.T) {
	s, _, a, p := childFixture(t, ui.ShowDefinition())
	previews := map[string]bool{}
	var mu sync.Mutex
	a.Sink = func(e agent.Event) {
		if e.Kind == "ui_preview_update" && e.UIPreview != nil {
			mu.Lock()
			previews[e.PreviewKey] = true
			mu.Unlock()
		}
	}
	turn := 0
	p.respond = func(llm.GenerationRequest) (llm.GenerationResult, error) {
		turn++
		if turn > 2 {
			return llm.GenerationResult{Text: "Both comparisons are ready."}, nil
		}
		widget := ui.Widget{Title: fmt.Sprintf("Comparison %d", turn), Fallback: fmt.Sprintf("Result %d", turn), Components: []ui.Component{{Name: "Table", Props: json.RawMessage(`{"title":"Result","columns":["Value"],"rows":[["42"]],"source":"Fixture"}`)}}}
		raw, _ := json.Marshal(widget)
		return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "repeated-provider-id", Name: ui.ShowName, Arguments: string(raw)}}}, nil
	}
	mustSubmit(t, a, "repeated-ui-call", "Show two comparisons")
	mustDrain(t, a)
	mu.Lock()
	defer mu.Unlock()
	if len(previews) != 2 || scalar(t, s, `SELECT count(*) FROM ui_widget WHERE message_id IS NOT NULL`) != 2 {
		t.Fatal("provider call ID overwrote another preview", previews)
	}
}
