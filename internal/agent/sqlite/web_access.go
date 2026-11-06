package sqlite

import (
	"context"
	"database/sql"

	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/plugin/webaccess"
	"github.com/z-chenhao/assistant/internal/tool"
	"github.com/z-chenhao/assistant/internal/webcontent"
)

func (r *Runs) ConfigureWebAccess(c webaccess.Config, p llm.LLMProvider, secrets ...llm.SecretStore) {
	r.webAccess = c
	if len(secrets) > 0 {
		r.webSecrets = secrets[0]
	}
	r.webFetch = webcontent.Fetch
	r.webContentFetch = webcontent.FetchContent
	r.webProvider = nil
	if p != nil {
		r.webProvider = r.GuardProvider(p)
	}
}

func (t *tools) executeWebAccess(ctx context.Context, inv tool.Invocation) (tool.Result, error) {
	var name, args string
	err := t.runs.write(ctx, func(tx *sql.Tx) error {
		_, n, a, e := t.acceptedBuiltin(ctx, tx, inv)
		name, args = n, a
		return e
	})
	if err != nil {
		return tool.Result{}, err
	}
	run, err := t.runs.Load(ctx, t.claim)
	if err != nil {
		return tool.Result{}, err
	}
	matched := false
	for _, s := range run.Tools {
		if s.Name == name && s.ResourceHash == t.runs.webAccess.Identity() {
			matched = true
		}
	}
	if !matched {
		return tool.Failure("MODEL_INCOMPATIBLE"), nil
	}
	plugin := webaccess.Plugin{Config: t.runs.webAccess, NativeSearch: run.Snapshot.ProviderKind == "openai_account", ContextWindow: run.Snapshot.ContextWindow, Secrets: t.runs.webSecrets, FetchContent: t.runs.webContentFetch, Fetch: t.runs.webFetch}
	return plugin.Execute(ctx, webAccessHost{tools: t, invocation: inv, run: run}, name, args)
}
