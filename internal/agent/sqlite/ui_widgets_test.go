package sqlite

import (
	"context"
	"encoding/json"
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
			return llm.GenerationResult{Text: "Explore the comparison."}, nil
		}
		doc, _ := json.Marshal(ui.Widget{Title: "Calculator", Fallback: "The result is 42.", HTML: `<button id="compute">Compute</button><output id="result"></output>`, JavaScript: `document.getElementById("compute").addEventListener("click",()=>document.getElementById("result").textContent=42)`})
		args, _ := json.Marshal(map[string]string{"source": "return await tools.builtin__intelligent_ui_show(" + string(doc) + ");"})
		return calls(codemode.ToolName, string(args)), nil
	}
	mustSubmit(t, a, "ui-widget", "Show the useful answer")
	mustDrain(t, a)
	if scalar(t, s, `SELECT count(*) FROM ui_widget WHERE message_id IS NOT NULL`) != 1 || scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE tool_name='builtin__intelligent_ui_show' AND status='succeeded' AND attempt_count=1`) != 1 {
		t.Fatal("widget missing or duplicated")
	}
	var ref, messageID string
	if err = s.db.QueryRow(`SELECT ui_ref,message_id FROM ui_widget`).Scan(&ref, &messageID); err != nil {
		t.Fatal(err)
	}
	widget, err := s.WebWidget(context.Background(), ref)
	if err != nil || widget.Title != "Calculator" {
		t.Fatal(widget, err)
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
		found = found || c.Type == "widget" && c.WidgetRef == ref
	}
	if !found {
		t.Fatal("reply lost UI reference", spec)
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
	calls := []llm.ToolCall{{ID: "show", Name: ui.ShowName, Arguments: `{"title":"Answer","fallback":"The answer is 42.","html":"<output>42</output>"}`}}
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
	if scalar(t, s, `SELECT count(*) FROM ui_widget`) != 1 || scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE status='succeeded'`) != 1 {
		t.Fatal("recovery duplicated the presentation")
	}
}
