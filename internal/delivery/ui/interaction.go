package ui

import (
	"encoding/json"
	"github.com/z-chenhao/assistant/internal/tool"
	"strings"
)

// ViewState is mutable, component-scoped presentation data. ModelContext is an
// explicit small user-data view; neither field carries Host authority.
type ViewState struct {
	Version      int64           `json:"version"`
	State        json.RawMessage `json:"state"`
	ModelContext json.RawMessage `json:"model_context"`
}

func ValidViewState(s ViewState) bool {
	return s.Version >= 0 && validStateJSON(s.State, 16<<10) && validStateJSON(s.ModelContext, 4096)
}
func validStateJSON(raw json.RawMessage, limit int) bool {
	if len(raw) > limit || len(raw) == 0 || raw[0] != '{' || !json.Valid(raw) {
		return false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	nodes := 0
	var visit func(any, int) bool
	visit = func(v any, depth int) bool {
		nodes++
		if depth > 8 || nodes > 2048 {
			return false
		}
		switch x := v.(type) {
		case map[string]any:
			for key, child := range x {
				if len(key) > 128 || key == "__proto__" || key == "constructor" || key == "prototype" || !visit(child, depth+1) {
					return false
				}
			}
		case []any:
			for _, child := range x {
				if !visit(child, depth+1) {
					return false
				}
			}
		case string:
			if len(x) > 8192 || strings.ContainsRune(x, 0) {
				return false
			}
		}
		return true
	}
	return visit(value, 0)
}

// ActionResult exposes a bounded accepted result, never execution ownership.
type ActionResult struct {
	Status       string       `json:"status"`
	MessageID    string       `json:"message_id,omitempty"`
	InvocationID string       `json:"invocation_id,omitempty"`
	Result       *tool.Result `json:"result,omitempty"`
}
