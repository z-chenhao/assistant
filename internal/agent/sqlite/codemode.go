package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/computer"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/tool"
	"github.com/z-chenhao/erii/internal/tool/codemode"
)

func (r *Runs) ConfigureJavaScript(runtime codemode.JavaScriptRuntime) { r.javascript = runtime }
func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

type codeBridge struct {
	t                 *tools
	req               codemode.ExecutionRequest
	attempt, exchange string
	registry          *tool.Registry
}
type nestedTools struct {
	*tools
	parent string
}

func (t nestedTools) Accept(ctx context.Context, exchange string, calls []llm.ToolCall, specs []tool.Spec) ([]tool.Invocation, error) {
	return t.tools.accept(ctx, exchange, calls, specs, t.parent)
}
func (t *tools) executeCode(ctx context.Context, inv tool.Invocation) (tool.Result, error) {
	if t.runs.javascript == nil {
		return tool.Failure("PROVIDER_UNAVAILABLE"), nil
	}
	var args struct {
		Source string `json:"source"`
	}
	if json.Unmarshal([]byte(inv.Call.Arguments), &args) != nil {
		return tool.Failure("INVALID_ARGUMENT"), nil
	}
	b := &codeBridge{t: t, attempt: newID()}
	req := codemode.ExecutionRequest{ProtocolVersion: codemode.ProtocolVersion, RuntimeVersion: codemode.RuntimeVersion, PreludeVersion: codemode.PreludeVersion, ExecutionID: inv.ID, ParentInvocationID: inv.ID, RunID: t.claim.RunID(), Source: args.Source, SourceHash: digest(args.Source), Limits: codemode.DefaultLimits()}
	err := t.runs.write(ctx, func(tx *sql.Tx) error {
		if err := fence(ctx, tx, t.claim); err != nil {
			return err
		}
		var raw string
		if err := tx.QueryRowContext(ctx, `SELECT r.kind,c.tools_json,i.provider_exchange_id FROM run r JOIN capability_snapshot c ON c.snapshot_id=r.capability_snapshot_id JOIN tool_invocation i USING(run_id) WHERE i.invocation_id=? AND i.status='running'`, inv.ID).Scan(&req.Kind, &raw, &b.exchange); err != nil {
			return err
		}
		if json.Unmarshal([]byte(raw), &req.Tools) != nil {
			return agent.ErrStorage
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO code_execution(execution_id,source_hash,runtime_version,prelude_version,created_at) VALUES(?,?,?,?,?) ON CONFLICT DO NOTHING`, inv.ID, req.SourceHash, req.RuntimeVersion, req.PreludeVersion, time.Now().UnixMicro())
		if err != nil {
			return err
		}
		var hash, runtime, prelude string
		if err = tx.QueryRowContext(ctx, `SELECT source_hash,runtime_version,prelude_version FROM code_execution WHERE execution_id=?`, inv.ID).Scan(&hash, &runtime, &prelude); err != nil {
			return err
		}
		if hash != req.SourceHash || runtime != req.RuntimeVersion || prelude != req.PreludeVersion {
			return tool.ErrReplay
		}
		return nil
	})
	if err != nil {
		return tool.Result{}, err
	}
	// Never recursively expose the execution tool as an inner Host capability.
	inner := []tool.Spec{}
	for _, spec := range req.Tools {
		if spec.Name != codemode.ToolName {
			inner = append(inner, spec)
		}
	}
	b.registry, err = t.runs.registry.Select(req.Tools)
	if err != nil {
		return tool.Result{}, err
	}
	req.Tools = inner
	b.req = req
	outcome, err := t.runs.javascript.Start(ctx, req, b)
	if err != nil {
		if ctx.Err() != nil {
			return tool.Result{}, ctx.Err()
		}
		return tool.Failure("PROVIDER_UNAVAILABLE"), nil
	}
	switch outcome.Kind {
	case "suspended":
		err = t.runs.write(ctx, func(tx *sql.Tx) error {
			if err := fence(ctx, tx, t.claim); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `UPDATE tool_invocation SET status='waiting',wait_reason='external_event',updated_at=?,version=version+1 WHERE invocation_id=? AND status='running'`, time.Now().UnixMicro(), inv.ID)
			return err
		})
		if err != nil {
			return tool.Result{}, err
		}
		return tool.Result{}, tool.ErrSuspended
	case "cancelled":
		return tool.Failure("CANCELLED"), nil
	case "failed":
		if outcome.Error != nil {
			return tool.Normalize(tool.Result{Error: outcome.Error}), nil
		}
		return tool.Failure("INVALID_ARGUMENT"), nil
	case "completed":
		value, _ := json.Marshal(outcome.Output)
		return tool.Result{Text: string(value)}, nil
	}
	return tool.Failure("PROVIDER_UNAVAILABLE"), nil
}
func (b *codeBridge) Handle(ctx context.Context, method string, m codemode.Message) (codemode.Reply, error) {
	empty := codemode.Reply{}
	if m.ExecutionID != b.req.ExecutionID || m.ParentInvocationID != b.req.ParentInvocationID || m.RunID != b.req.RunID {
		return empty, agent.ErrConflict
	}
	// Persist every incoming message, including frozen Host parameters, before ack.
	err := b.t.runs.write(ctx, func(tx *sql.Tx) error {
		if err := fence(ctx, tx, b.t.claim); err != nil {
			return err
		}
		value, _ := encoded(m)
		var old string
		err := tx.QueryRowContext(ctx, `SELECT message_json FROM code_event WHERE execution_id=? AND attempt_id=? AND sequence=?`, m.ExecutionID, b.attempt, m.Sequence).Scan(&old)
		if err == nil {
			if old != value {
				return tool.ErrReplay
			}
			return nil
		}
		if err != sql.ErrNoRows {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO code_event VALUES(?,?,?,?)`, m.ExecutionID, b.attempt, m.Sequence, value); err != nil {
			return err
		}
		if method == "execution.event" {
			return runEvent(ctx, tx, b.t.claim.RunID(), "codemode."+m.Kind, 1)
		}
		return nil
	})
	if err != nil {
		return empty, err
	}
	switch method {
	case "execution.event":
		return empty, nil
	case "host.invoke":
		n, err := strconv.Atoi(m.CallID)
		if err != nil || n < 1 || n > 10000 {
			return denied("PERMISSION_DENIED"), nil
		}
		if m.ToolName == codemode.ToolName {
			return denied("PERMISSION_DENIED"), nil
		}
		visible := false
		for _, s := range b.req.Tools {
			visible = visible || s.Name == m.ToolName
		}
		if !visible {
			return denied("PERMISSION_DENIED"), nil
		}
		// IDs are generated in trusted Runtime code; script arguments cannot select them.
		arguments, err := llm.CanonicalJSON(m.Arguments)
		if err != nil {
			return denied("INVALID_ARGUMENT"), nil
		}
		call := llm.ToolCall{ID: b.req.ExecutionID + "/" + m.CallID, Name: m.ToolName, Arguments: string(arguments)}
		results, err := (tool.Executor{Registry: b.registry, Store: nestedTools{b.t, b.req.ExecutionID}, Concurrency: 1}).Execute(computer.WithExecution(ctx, b.req.ExecutionID), b.exchange, []llm.ToolCall{call})
		if errors.Is(err, tool.ErrSuspended) {
			return codemode.Reply{Suspended: true}, nil
		}
		if errors.Is(err, tool.ErrReplay) {
			return denied("NON_DETERMINISTIC_REPLAY"), nil
		}
		if err != nil {
			return empty, err
		}
		r := results[0]
		if r.Error != nil {
			return codemode.Reply{Error: r.Error}, nil
		}
		// Providers normalize their business result to JSON text at ToolExecutor.
		var value any
		if json.Unmarshal([]byte(r.Text), &value) != nil {
			value = r.Text
		}
		if len(r.ArtifactRefs) > 0 {
			value = map[string]any{"text": r.Text, "artifactRefs": r.ArtifactRefs}
		}
		return codemode.Reply{Value: value}, nil
	case "host.store_get", "host.store_put":
		return b.store(ctx, method, m.Arguments)
	case "host.discover":
		return b.discover(m), nil
	}
	return denied("PERMISSION_DENIED"), nil
}
func denied(code string) codemode.Reply { return codemode.Reply{Error: tool.Failure(code).Error} }
func (b *codeBridge) store(ctx context.Context, method string, raw json.RawMessage) (codemode.Reply, error) {
	var args struct {
		Key   string          `json:"key"`
		Value json.RawMessage `json:"value"`
	}
	if json.Unmarshal(raw, &args) != nil || len(args.Key) == 0 || len(args.Key) > 256 {
		return denied("INVALID_ARGUMENT"), nil
	}
	reply := codemode.Reply{}
	err := b.t.runs.write(ctx, func(tx *sql.Tx) error {
		if err := fence(ctx, tx, b.t.claim); err != nil {
			return err
		}
		if method == "host.store_put" {
			if !json.Valid(args.Value) || len(args.Value) > 262144 || strings.Contains(string(args.Value), `"handle_token"`) || strings.Contains(string(args.Value), `"handleToken"`) {
				reply = denied("INVALID_ARGUMENT")
				return nil
			}
			var size int
			if err := tx.QueryRowContext(ctx, `SELECT coalesce(sum(length(CAST(value_json AS BLOB))+length(key)),0) FROM code_store WHERE execution_id=? AND key!=?`, b.req.ExecutionID, args.Key).Scan(&size); err != nil {
				return err
			}
			if size+len(args.Value)+len(args.Key) > 1<<20 {
				reply = denied("RESOURCE_LIMIT")
				return nil
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO code_store VALUES(?,?,?,1) ON CONFLICT(execution_id,key) DO UPDATE SET value_json=excluded.value_json,version=code_store.version+1`, b.req.ExecutionID, args.Key, string(args.Value))
			return err
		}
		var value string
		err := tx.QueryRowContext(ctx, `SELECT value_json FROM code_store WHERE execution_id=? AND key=?`, b.req.ExecutionID, args.Key).Scan(&value)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		reply.Value = json.RawMessage(value)
		return nil
	})
	return reply, err
}
func (b *codeBridge) discover(m codemode.Message) codemode.Reply {
	type description struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		InputSchema string `json:"inputSchema,omitempty"`
	}
	switch m.ToolName {
	case "describeTool":
		var name string
		if json.Unmarshal(m.Arguments, &name) != nil {
			return denied("INVALID_ARGUMENT")
		}
		for _, s := range b.req.Tools {
			if s.Name == name {
				return codemode.Reply{Value: description{s.Name, s.Description, s.InputSchema}}
			}
		}
	case "describeNamespace":
		var name string
		if json.Unmarshal(m.Arguments, &name) != nil {
			return denied("INVALID_ARGUMENT")
		}
		if name == "cua" {
			return codemode.Reply{Value: map[string]any{"name": "cua", "description": "Discover inventory with getState/listApps/listWindows/listBrowsers/listTabs. Bind with getApp/getBrowser/getTab/createBrowserTab. Bound surfaces expose documentation, observe and capabilities. Re-observe after every action. Handles expire with the cell."}}
		}
		if name == "agent" {
			for _, s := range b.req.Tools {
				if s.Name == agent.RunToolName {
					return codemode.Reply{Value: map[string]any{"name": "agent", "description": "run({task, artifactRefs?}) returns Promise<{text, artifactRefs}>. Creates one general Child Worker; does not create a Task. Host-accepted calls continue after script end; errors reject normally."}}
				}
			}
		}
		items := []description{}
		for _, s := range b.req.Tools {
			if strings.HasPrefix(s.Name, name+"__") {
				items = append(items, description{s.Name, s.Description, ""})
			}
		}
		if len(items) > 0 {
			return codemode.Reply{Value: items}
		}
	case "searchTools":
		var args []json.RawMessage
		if json.Unmarshal(m.Arguments, &args) != nil || len(args) == 0 {
			return denied("INVALID_ARGUMENT")
		}
		var query string
		if json.Unmarshal(args[0], &query) != nil {
			return denied("INVALID_ARGUMENT")
		}
		options := struct {
			Limit     int    `json:"limit"`
			Namespace string `json:"namespace"`
		}{Limit: 20}
		if len(args) > 1 && json.Unmarshal(args[1], &options) != nil {
			return denied("INVALID_ARGUMENT")
		}
		if options.Limit < 1 || options.Limit > 100 {
			return denied("INVALID_ARGUMENT")
		}
		items := []description{}
		for _, s := range b.req.Tools {
			if strings.Contains(strings.ToLower(s.Name+" "+s.Description), strings.ToLower(query)) && (options.Namespace == "" || strings.HasPrefix(s.Name, options.Namespace+"__")) {
				items = append(items, description{s.Name, s.Description, ""})
				if len(items) >= options.Limit {
					break
				}
			}
		}
		return codemode.Reply{Value: items}
	}
	return codemode.Reply{}
}
