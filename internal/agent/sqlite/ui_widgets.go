package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/delivery/ui"
	"github.com/z-chenhao/assistant/internal/tool"
)

func (t *tools) showUI(ctx context.Context, inv tool.Invocation) (tool.Result, error) {
	var result tool.Result
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
		if err = tx.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(CASE WHEN json_array_length(document_json,'$.components')>0 THEN json_array_length(document_json,'$.components') ELSE 1 END),0),(SELECT count(*) FROM ui_widget w,json_each(w.document_json,'$.components') c WHERE w.run_id=? AND w.message_id IS NULL AND w.invocation_id!=? AND json_extract(c.value,'$.type')='form') FROM ui_widget WHERE run_id=? AND message_id IS NULL AND invocation_id!=?`, t.claim.RunID(), inv.ID, t.claim.RunID(), inv.ID).Scan(&count, &components, &forms); err != nil {
			return err
		}
		added := max(1, len(widget.Components))
		for _, c := range widget.Components {
			if c.Type == "form" {
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
		return nil
	})
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
		w, ok := ui.ParseWidget(raw)
		if !ok {
			return spec, agent.ErrStorage
		}
		spec.Components = append(spec.Components, ui.WidgetComponents(w, ref, message, len(spec.Components))...)
	}
	return spec, rows.Err()
}

// WebWidget reads only a delivered Main-owned artifact in the current account.
func (s *Store) WebWidget(ctx context.Context, ref string) (ui.Widget, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT w.document_json FROM ui_widget w JOIN run r USING(run_id) JOIN conversation_intent c ON c.message_id=w.message_id JOIN tool_invocation i USING(invocation_id) WHERE w.ui_ref=? AND r.kind='main' AND r.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1) AND i.status='succeeded'`, ref).Scan(&raw)
	if err != nil {
		return ui.Widget{}, agent.ErrNotFound
	}
	w, ok := ui.ParseWidget(raw)
	if !ok {
		return w, agent.ErrStorage
	}
	return w, nil
}
