package agent

import (
	"encoding/json"

	"github.com/z-chenhao/assistant/internal/tool"
)

// Tool declarations change only when the Host's actual search result is
// checkpointed with its delivery. Pending receipts and user text cannot load
// tools. The independent frozen capability remains the authorization boundary.
func applyToolLoads(cp *Checkpoint, specs []tool.Spec, name string, result tool.Result) error {
	if name != tool.SearchName || result.Error != nil {
		return nil
	}
	var found tool.SearchResult
	if json.Unmarshal([]byte(result.Text), &found) != nil {
		return nil
	}
	if len(found.Loaded) > tool.MaxSearchLimit {
		return ErrStorage
	}
	allowed, active := map[string]bool{}, map[string]bool{}
	names := append([]string(nil), cp.LoadedTools...)
	for _, spec := range specs {
		if spec.Searchable() {
			allowed[spec.Name] = true
		}
	}
	for _, name := range cp.LoadedTools {
		if !allowed[name] {
			return ErrStorage
		}
		active[name] = true
	}
	for _, name := range found.Loaded {
		if !allowed[name] {
			return ErrStorage
		}
		if !active[name] {
			names = append(names, name)
			active[name] = true
		}
	}
	cp.LoadedTools = names
	return nil
}
