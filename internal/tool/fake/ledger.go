// Package fake supplies an explicit external-effect ledger for deterministic CI.
// It is never enabled by configuration or by a missing provider/credential.
package fake

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"

	"github.com/z-chenhao/erii/internal/tool"
	_ "modernc.org/sqlite"
)

const Name = "mcp__fixture__send"
const Schema = `{"type":"object","properties":{"target":{"type":"string","minLength":1},"text":{"type":"string","maxLength":4096},"behavior":{"enum":["success","ambiguous","transient","error","unknown","timeout"]}},"required":["target","text"],"additionalProperties":false}`

type arguments struct{ Target, Text, Behavior string }

func Definition(root, policy string, boundary func(string)) tool.Definition {
	open := func() (*sql.DB, error) {
		db, err := sql.Open("sqlite", filepath.Join(root, "effect-ledger.db")+"?_pragma=busy_timeout(5000)")
		if err != nil {
			return nil, err
		}
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS effects(key TEXT PRIMARY KEY,args TEXT NOT NULL,result TEXT NOT NULL)`)
		if err != nil {
			db.Close()
			return nil, err
		}
		return db, nil
	}
	d := tool.Definition{Spec: tool.Spec{Name: Name, Version: 1, InputSchema: Schema, Permission: "external_effect", Risk: "high", ReplayPolicy: policy, TimeoutMS: 1000, OutputBytes: 16384, Provider: "fixture", ConflictPolicy: "target-v1"}, ConflictKeys: func(raw string) []string {
		var a arguments
		_ = json.Unmarshal([]byte(raw), &a)
		return []string{"fixture:target:" + a.Target}
	}}
	d.Invoke = func(ctx context.Context, request tool.Request) tool.Outcome {
		var a arguments
		if json.Unmarshal([]byte(request.Arguments), &a) != nil {
			return tool.Outcome{Result: tool.Failure("INVALID_ARGUMENT")}
		}
		if a.Behavior == "transient" {
			return tool.Outcome{Result: tool.Result{Error: &tool.Error{Code: "PROVIDER_UNAVAILABLE", Message: "raw provider credential=secret /private/path"}}, Transient: true}
		}
		if a.Behavior == "error" {
			return tool.Outcome{Result: tool.Result{Error: &tool.Error{Code: "INVALID_ARGUMENT", Message: "raw provider SQL secret internal-id"}}}
		}
		if a.Behavior == "unknown" {
			return tool.Outcome{Ambiguous: true}
		}
		if a.Behavior == "timeout" {
			<-ctx.Done()
			return tool.Outcome{Ambiguous: true}
		}
		if boundary != nil {
			boundary("before_provider_effect")
		}
		db, err := open()
		if err != nil {
			return tool.Outcome{Result: tool.Failure("PROVIDER_UNAVAILABLE"), Transient: true}
		}
		defer db.Close()
		result := "Effect recorded for " + a.Target + ": " + a.Text
		_, err = db.ExecContext(ctx, `INSERT INTO effects VALUES(?,?,?) ON CONFLICT(key) DO NOTHING`, request.IdempotencyKey, request.Arguments, result)
		if err != nil {
			return tool.Outcome{Ambiguous: true}
		}
		if boundary != nil {
			boundary("after_provider_effect")
		}
		if a.Behavior == "ambiguous" {
			return tool.Outcome{Ambiguous: true}
		}
		return tool.Outcome{Result: tool.Result{Text: result}}
	}
	// Even a never-replay tool may expose authoritative reconciliation evidence.
	d.Query = func(ctx context.Context, request tool.Request) tool.Evidence {
		var a arguments
		_ = json.Unmarshal([]byte(request.Arguments), &a)
		if a.Behavior == "unknown" || a.Behavior == "timeout" {
			return tool.Evidence{State: "unknown"}
		}
		db, err := open()
		if err != nil {
			return tool.Evidence{State: "unknown"}
		}
		defer db.Close()
		var args, result string
		err = db.QueryRowContext(ctx, `SELECT args,result FROM effects WHERE key=?`, request.IdempotencyKey).Scan(&args, &result)
		if err == sql.ErrNoRows {
			return tool.Evidence{State: "absent"}
		}
		if err != nil || args != request.Arguments {
			return tool.Evidence{State: "unknown"}
		}
		return tool.Evidence{State: "succeeded", Result: tool.Result{Text: result}}
	}
	return d
}
