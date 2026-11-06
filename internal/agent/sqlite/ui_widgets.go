package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/conversation"
	"github.com/z-chenhao/assistant/internal/delivery"
	"github.com/z-chenhao/assistant/internal/delivery/ui"
	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/tool"
)

func (t *tools) showUI(ctx context.Context, inv tool.Invocation) (tool.Result, error) {
	var result tool.Result
	var previewInput string
	err := t.runs.write(ctx, func(tx *sql.Tx) error {
		kind, name, args, err := t.acceptedBuiltin(ctx, tx, inv)
		if err != nil {
			return err
		}
		if kind != "main" || name != ui.ShowName {
			result = tool.Failure("PERMISSION_DENIED")
			return nil
		}
		widget, ok := ui.ParseWidget(args)
		if !ok {
			result = tool.Failure("INVALID_ARGUMENT")
			return nil
		}
		var count, components, forms int
		if err = tx.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(CASE WHEN json_array_length(document_json,'$.components')>0 THEN json_array_length(document_json,'$.components') ELSE 1 END),0),(SELECT count(*) FROM ui_widget w,json_each(w.document_json,'$.components') c WHERE w.run_id=? AND w.message_id IS NULL AND w.invocation_id!=? AND (json_extract(c.value,'$.type')='form' OR json_extract(c.value,'$.name') IN ('scheduleTime','steward.form'))) FROM ui_widget WHERE run_id=? AND message_id IS NULL AND invocation_id!=?`, t.claim.RunID(), inv.ID, t.claim.RunID(), inv.ID).Scan(&count, &components, &forms); err != nil {
			return err
		}
		added := max(1, len(widget.Components))
		for _, c := range widget.Components {
			if c.Name == "scheduleTime" || c.Name == "steward.form" {
				forms++
			}
		}
		if count >= 16 || components+added > 96 || forms > 4 {
			result = tool.Failure("RESOURCE_LIMIT")
			return nil
		}
		ref := "ui:" + inv.ID
		if _, err = tx.ExecContext(ctx, `INSERT INTO ui_widget(invocation_id,run_id,ui_ref,document_json,created_at) VALUES(?,?,?,?,?) ON CONFLICT(invocation_id) DO NOTHING`, inv.ID, t.claim.RunID(), ref, args, time.Now().UnixMicro()); err != nil {
			return err
		}
		data, _ := json.Marshal(map[string]string{"ui_ref": ref, "status": "staged"})
		result = tool.Result{Text: string(data)}
		previewInput = args
		return nil
	})
	if err == nil && result.Error == nil {
		ui.ObservePreview(ctx, inv.ID, previewInput)
	}
	return result, err
}

type uiQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func appendWidgets(ctx context.Context, q uiQuerier, message string, spec ui.Spec) (ui.Spec, error) {
	rows, err := q.QueryContext(ctx, `SELECT ui_ref,document_json FROM ui_widget w JOIN tool_invocation i USING(invocation_id) WHERE w.message_id=? AND i.status='succeeded' ORDER BY i.delivery_seq`, message)
	if err != nil {
		return spec, err
	}
	defer rows.Close()
	for rows.Next() {
		var ref, raw string
		if err = rows.Scan(&ref, &raw); err != nil {
			return spec, err
		}
		w, ok := ui.StoredWidget(raw)
		if !ok {
			return spec, agent.ErrStorage
		}
		spec.Components = append(spec.Components, ui.WidgetComponents(w, ref, message, len(spec.Components))...)
	}
	return spec, rows.Err()
}

// WebWidget reads only a delivered Main-owned artifact in the current account.
func (s *Store) WebWidget(ctx context.Context, ref string) (ui.Widget, error) {
	base, index, found := strings.Cut(ref, "#")
	if !found {
		return ui.Widget{}, agent.ErrNotFound
	}
	i, err := strconv.Atoi(index)
	if err != nil || i < 0 || strconv.Itoa(i) != index {
		return ui.Widget{}, agent.ErrNotFound
	}
	var raw string
	err = s.db.QueryRowContext(ctx, `SELECT w.document_json FROM ui_widget w JOIN run r USING(run_id) JOIN conversation_intent c ON c.message_id=w.message_id JOIN tool_invocation i USING(invocation_id) WHERE w.ui_ref=? AND r.kind='main' AND r.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1) AND i.status='succeeded'`, base).Scan(&raw)
	if err != nil {
		return ui.Widget{}, agent.ErrNotFound
	}
	w, ok := ui.StoredWidget(raw)
	if !ok {
		return w, agent.ErrStorage
	}
	if i >= len(w.Components) || w.Components[i].Name != "generateSandboxedUi" {
		return ui.Widget{}, agent.ErrNotFound
	}
	w.Components = w.Components[i : i+1]
	return w, nil
}

// WebBubbles splits only a projection whose frozen parts match the bound UI
// artifacts. Stored Conversation and Delivery projections remain untouched.
// Slices share the original components, including submitted-form restoration.
func (s *Store) WebBubbles(ctx context.Context, m conversation.Message, spec ui.Spec) ([]ui.Spec, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ui_ref,document_json FROM ui_widget w JOIN tool_invocation i USING(invocation_id) WHERE w.message_id=? AND i.status='succeeded' ORDER BY i.delivery_seq`, m.MessageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type bound struct {
		ref    string
		widget ui.Widget
	}
	var items []bound
	for rows.Next() {
		var ref, raw string
		if err := rows.Scan(&ref, &raw); err != nil {
			return nil, err
		}
		w, ok := ui.StoredWidget(raw)
		if !ok {
			return nil, agent.ErrStorage
		}
		items = append(items, bound{ref, w})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 || len(m.Content) < len(items) {
		return nil, nil
	}
	start := len(m.Content) - len(items)
	for i, item := range items {
		part := m.Content[start+i]
		if part.Type != "text" || part.Text != item.widget.Fallback {
			return nil, nil
		}
	}
	base := ui.Compose(delivery.Envelope{Message: m})
	if len(spec.Components) < len(base.Components) || ui.Hash(spec.Components[:len(base.Components)]) != ui.Hash(base.Components) {
		return nil, nil
	}
	offset := len(base.Components)
	var bubbles []ui.Spec
	for _, item := range items {
		expected := ui.WidgetComponents(item.widget, item.ref, m.MessageID, offset)
		end := offset + len(expected)
		if end > len(spec.Components) || ui.Hash(spec.Components[offset:end]) != ui.Hash(expected) {
			return nil, nil
		}
		for j := range expected {
			if expected[j].Name != "" {
				spec.Components[offset+j].WidgetRef = item.ref + "#" + strconv.Itoa(j)
			}
		}
		bubbles = append(bubbles, ui.Spec{SchemaVersion: ui.Version, Fallback: item.widget.Fallback, Components: spec.Components[offset:end]})
		offset = end
	}
	if offset != len(spec.Components) {
		return nil, nil
	}
	m.Content = m.Content[:start]
	reply := ui.Compose(delivery.Envelope{Message: m})
	if len(reply.Components) > 0 {
		bubbles = append(bubbles, ui.Spec{SchemaVersion: ui.Version, Fallback: reply.Fallback, Components: spec.Components[:len(reply.Components)]})
	}
	return bubbles, nil
}

// StagedUIPreviews reads successful unbound UI artifacts under the active claim.
func (r *Runs) StagedUIPreviews(ctx context.Context, c agent.RunClaim) ([]llm.ToolCall, error) {
	var calls []llm.ToolCall
	err := r.write(ctx, func(tx *sql.Tx) error {
		if err := fence(ctx, tx, c); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT i.invocation_id,w.document_json FROM ui_widget w JOIN tool_invocation i USING(invocation_id) WHERE w.run_id=? AND w.message_id IS NULL AND i.status='succeeded' ORDER BY i.delivery_seq LIMIT 16`, c.RunID())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var call llm.ToolCall
			if err := rows.Scan(&call.ID, &call.Arguments); err != nil {
				return err
			}
			calls = append(calls, call)
		}
		return rows.Err()
	})
	return calls, err
}
