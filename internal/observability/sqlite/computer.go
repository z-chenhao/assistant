package sqlite

import (
	"context"
	"time"

	"github.com/z-chenhao/collie/internal/computer"
)

// ComputerActivity reads committed Tool metadata only, scoped to the configured
// agent. A preview capture is deliberately not recorded as agent activity.
func (s *Inspector) ComputerActivity(ctx context.Context) ([]computer.Activity, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT i.invocation_id, CASE WHEN i.tool_name='builtin__computer_discover' THEN 'getState' ELSE coalesce(substr(json_extract(i.arguments_json,'$.method'),1,80),'Computer operation') END, i.status, i.updated_at
 FROM tool_invocation i JOIN run r ON r.run_id=i.run_id
 WHERE r.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1) AND i.tool_name IN ('builtin__computer_discover','builtin__computer_bind','builtin__computer_invoke')
 ORDER BY i.updated_at DESC,i.invocation_id DESC LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []computer.Activity{}
	for rows.Next() {
		var item computer.Activity
		var at int64
		if err := rows.Scan(&item.ID, &item.Action, &item.Status, &at); err != nil {
			return nil, err
		}
		switch item.Action {
		case "getState", "getApp", "getTab", "getBrowser", "createBrowserTab", "getScreenshot", "getAXState", "getAXStateAndScreenshot", "observe", "click", "typeText", "scroll", "pressKey", "key", "move", "drag", "paste", "setValue", "getClipboard", "downloadFile", "uploadFile":
		default:
			item.Action = "Computer operation"
		}
		item.At = time.UnixMicro(at).UTC().Format(time.RFC3339Nano)
		items = append(items, item)
	}
	return items, rows.Err()
}
