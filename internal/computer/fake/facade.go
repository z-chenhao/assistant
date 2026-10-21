// Package fake supplies deterministic Computer inventory and leased surfaces.
// It is selected only by explicit test Bridge injection, never by user config.
package fake

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/google/uuid"
	"github.com/z-chenhao/erii/internal/computer"
	"github.com/z-chenhao/erii/internal/tool"
)

func Definitions() []tool.Definition {
	var mu sync.Mutex
	leases := map[string]string{}
	result := func(value any) tool.Result { b, _ := json.Marshal(value); return tool.Result{Text: string(b)} }
	definitions := []tool.Definition{}
	for _, operation := range []string{"discover", "bind", "invoke"} {
		operation := operation
		definitions = append(definitions, tool.Definition{Spec: tool.Spec{Name: "builtin__computer__" + operation, Version: 1, Description: "Computer " + operation + " bridge; use cua facade.", InputSchema: `{"type":"object"}`, Permission: "read_only", ReplayPolicy: "safe"}, Execute: func(ctx context.Context, args string) tool.Result {
			scope := computer.Execution(ctx)
			if scope == "" {
				return tool.Failure("PERMISSION_DENIED")
			}
			var input struct {
				Method string          `json:"method"`
				Token  string          `json:"handle_token"`
				Args   json.RawMessage `json:"args"`
			}
			if json.Unmarshal([]byte(args), &input) != nil {
				return tool.Failure("INVALID_ARGUMENT")
			}
			mu.Lock()
			defer mu.Unlock()
			switch operation {
			case "discover":
				switch input.Method {
				case "getState":
					return result(map[string]any{"computerId": "fixture-computer", "instanceEpoch": "1", "platform": "macos", "apps": []any{map[string]string{"id": "notes", "name": "Notes"}}, "windows": []any{}, "browsers": []any{}})
				case "listApps":
					return result([]any{map[string]string{"id": "notes", "name": "Notes"}})
				case "listWindows", "listBrowsers", "listTabs":
					return result([]any{})
				}
			case "bind":
				switch input.Method {
				case "getApp", "getTab", "getBrowser", "createBrowserTab":
					token := uuid.NewString()
					leases[token] = scope
					return result(map[string]string{"handle_token": token})
				}
			case "invoke":
				if leases[input.Token] != scope {
					return tool.Failure("PERMISSION_DENIED")
				}
				switch input.Method {
				case "observe":
					return result(map[string]any{"observationId": "observation-v1", "text": "Fixture Notes ready", "elements": []any{map[string]string{"ref": "notes-v1", "label": "Notes"}}})
				case "documentation":
					return result(map[string]string{"text": "observe() returns current observation. Read-only Fake; no actions."})
				case "capabilities.list":
					return result([]string{"observe", "documentation"})
				case "capabilities.get", "capabilities.documentation":
					return result(map[string]string{"text": "observe()"})
				}
			}
			return tool.Failure("NOT_FOUND")
		}})
	}
	return definitions
}
