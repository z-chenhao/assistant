package sqlite

import (
	"context"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/automation"
	"github.com/z-chenhao/erii/internal/memory"
	"github.com/z-chenhao/erii/internal/tool"
	"github.com/z-chenhao/erii/internal/tool/codemode"
)

// ExecuteBuiltin routes the accepted, persisted Tool name through the shared
// Host execution pipeline. Each capability retains its own transaction/fencing.
func (t *tools) ExecuteBuiltin(ctx context.Context, inv tool.Invocation) (tool.Result, error) {
	var name string
	if err := t.runs.host.db.QueryRowContext(ctx, `SELECT tool_name FROM tool_invocation WHERE invocation_id=? AND run_id=?`, inv.ID, t.claim.RunID()).Scan(&name); err != nil {
		return tool.Result{}, agent.ErrStorage
	}
	if name == automation.ManageName || name == automation.ObserveName {
		return t.executeAutomation(ctx, inv)
	}
	if name == codemode.ToolName {
		return t.executeCode(ctx, inv)
	}
	if name == "builtin__computer__discover" || name == "builtin__computer__bind" || name == "builtin__computer__invoke" {
		return t.executeComputer(ctx, inv)
	}
	if name == agent.RunToolName {
		return t.AgentToolExecutor(ctx, inv.ID)
	}
	if name == memory.ManageName {
		return t.manageMemory(ctx, inv)
	}
	if name == "builtin__memory__search" {
		return t.searchMemory(ctx, inv)
	}
	return t.executeTaskAdmission(ctx, inv)
}
