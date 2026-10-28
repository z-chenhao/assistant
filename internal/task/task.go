// Package task owns the canonical Main-only Task control contract.
package task

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/z-chenhao/erii/internal/tool"
)

const SearchName = "builtin__task_search"
const ManageName = "builtin__task_manage"
const SearchSchema = `{"type":"object","properties":{"query":{"type":"string"},"scope":{"type":"string","enum":["active","recent","all"]},"status":{"type":"array","items":{"type":"string","enum":["active","paused","completed","cancelled"]}},"limit":{"type":"integer","minimum":1,"maximum":50}},"additionalProperties":false}`
const ManageSchema = `{"oneOf":[{"type":"object","properties":{"operation":{"const":"create"},"task":{"type":"string"},"completionPolicy":{"type":"string","enum":["once","ongoing"],"description":"Defaults to once. Use ongoing for continuing work; ordinary Root completion keeps the Task active."},"artifactRefs":{"type":"array","items":{"type":"string"}}},"required":["operation","task"],"additionalProperties":false},{"type":"object","properties":{"operation":{"const":"append"},"taskRef":{"type":"string"},"instruction":{"type":"string"},"artifactRefs":{"type":"array","items":{"type":"string"}}},"required":["operation","taskRef","instruction"],"additionalProperties":false},{"type":"object","properties":{"operation":{"const":"pause"},"taskRef":{"type":"string"}},"required":["operation","taskRef"],"additionalProperties":false},{"type":"object","properties":{"operation":{"const":"cancel"},"taskRef":{"type":"string"}},"required":["operation","taskRef"],"additionalProperties":false},{"type":"object","properties":{"operation":{"const":"complete"},"taskRef":{"type":"string"}},"required":["operation","taskRef"],"additionalProperties":false}]}`

func Definitions() []tool.Definition {
	return []tool.Definition{
		{Spec: tool.Spec{Name: SearchName, Version: 1, InputSchema: SearchSchema, ReplayPolicy: "safe", Permission: "read_only"}, Host: true},
		{Spec: tool.Spec{Name: ManageName, Version: 1, Description: "Create or control a Task. Create defaults to once; ongoing keeps continuing work active after a Root returns. Append continues work with a new Root when needed. Complete ends only an active Task with a terminal current-epoch tree and no unresolved Tool effects; it never stops work.", InputSchema: ManageSchema, ReplayPolicy: "safe", Permission: "task_control"}, Host: true},
	}
}
func Registry() *tool.Registry {
	r, err := tool.NewRegistry(Definitions())
	if err != nil {
		panic(err)
	}
	return r
}

type ManageRequest struct {
	Operation        string   `json:"operation"`
	Task             string   `json:"task,omitempty"`
	TaskRef          string   `json:"taskRef,omitempty"`
	Instruction      string   `json:"instruction,omitempty"`
	ArtifactRefs     []string `json:"artifactRefs,omitempty"`
	CompletionPolicy string   `json:"completionPolicy,omitempty"`
}
type SearchRequest struct {
	Query  string   `json:"query,omitempty"`
	Scope  string   `json:"scope,omitempty"`
	Status []string `json:"status,omitempty"`
	Limit  int      `json:"limit,omitempty"`
}
type Receipt struct {
	TaskRef  string `json:"taskRef"`
	Status   string `json:"status"`
	Accepted bool   `json:"accepted"`
}
type Summary struct {
	TaskRef         string `json:"taskRef"`
	Title           string `json:"title"`
	GoalSummary     string `json:"goalSummary"`
	Status          string `json:"status"`
	ProgressSummary string `json:"progressSummary,omitempty"`
	UpdatedAt       string `json:"updatedAt"`
}
type UpdateDetails struct {
	TaskRef      string   `json:"task_ref"`
	Kind         string   `json:"kind"`
	ArtifactRefs []string `json:"artifact_refs,omitempty"`
}

var ErrInvalid = errors.New("invalid task request")

// DecodeObject rejects duplicate keys and explicit nulls as well as unknown
// fields. Validating the discriminator separately prevents cross-arm fields.
func object(raw string) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil, ErrInvalid
	}
	fields := map[string]json.RawMessage{}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, ErrInvalid
		}
		k, ok := key.(string)
		if !ok {
			return nil, ErrInvalid
		}
		if _, ok = fields[k]; ok {
			return nil, ErrInvalid
		}
		var v json.RawMessage
		if dec.Decode(&v) != nil || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return nil, ErrInvalid
		}
		fields[k] = v
	}
	if _, err = dec.Token(); err != nil {
		return nil, ErrInvalid
	}
	if _, err = dec.Token(); err != io.EOF {
		return nil, ErrInvalid
	}
	return fields, nil
}
func validText(v string) bool {
	return strings.TrimSpace(v) != "" && utf8.ValidString(v) && len(v) <= 16<<10
}
func ParseManage(raw string) (ManageRequest, error) {
	var r ManageRequest
	fields, err := object(raw)
	if err != nil || json.Unmarshal([]byte(raw), &r) != nil {
		return r, ErrInvalid
	}
	allowed := map[string]bool{"operation": true, "taskRef": true}
	switch r.Operation {
	case "create":
		allowed = map[string]bool{"operation": true, "task": true, "artifactRefs": true, "completionPolicy": true}
		if _, present := fields["completionPolicy"]; present && r.CompletionPolicy != "once" && r.CompletionPolicy != "ongoing" {
			return r, ErrInvalid
		}
		if !validText(r.Task) {
			return r, ErrInvalid
		}
	case "append":
		allowed["instruction"] = true
		allowed["artifactRefs"] = true
		if !validText(r.Instruction) {
			return r, ErrInvalid
		}
	case "pause", "cancel", "complete":
	default:
		return r, ErrInvalid
	}
	for k := range fields {
		if !allowed[k] {
			return r, ErrInvalid
		}
	}
	if r.Operation != "create" && (!validText(r.TaskRef) || len(r.TaskRef) > 256) {
		return r, ErrInvalid
	}
	if len(r.ArtifactRefs) > 128 {
		return r, ErrInvalid
	}
	for _, ref := range r.ArtifactRefs {
		if !validText(ref) || len(ref) > 256 {
			return r, ErrInvalid
		}
	}
	if r.ArtifactRefs == nil {
		r.ArtifactRefs = []string{}
	}
	// Omitted and explicit once are the same action. Preserve legacy receipt hashes.
	if r.CompletionPolicy == "once" {
		r.CompletionPolicy = ""
	}
	return r, nil
}
func ParseSearch(raw string) (SearchRequest, error) {
	var r SearchRequest
	fields, err := object(raw)
	if err != nil || json.Unmarshal([]byte(raw), &r) != nil {
		return r, ErrInvalid
	}
	for k := range fields {
		if k != "query" && k != "scope" && k != "status" && k != "limit" {
			return r, ErrInvalid
		}
	}
	if len(r.Query) > 4096 {
		return r, ErrInvalid
	}
	if r.Scope == "" {
		if _, ok := fields["scope"]; ok {
			return r, ErrInvalid
		}
		r.Scope = "active"
		if r.Query != "" {
			r.Scope = "all"
		}
	}
	if r.Scope != "active" && r.Scope != "recent" && r.Scope != "all" {
		return r, ErrInvalid
	}
	if _, ok := fields["limit"]; !ok {
		r.Limit = 20
	}
	if r.Limit < 1 || r.Limit > 50 {
		return r, ErrInvalid
	}
	for _, s := range r.Status {
		if s != "active" && s != "paused" && s != "completed" && s != "cancelled" {
			return r, ErrInvalid
		}
	}
	return r, nil
}
func Title(text string) string {
	r := []rune(strings.Join(strings.Fields(text), " "))
	if len(r) > 80 {
		r = r[:80]
	}
	return string(r)
}

// BoundSummary keeps the maximum 50-row result below the runtime ContentPart
// limit, including worst-case JSON escaping. It never changes stored anchors.
func BoundSummary(s Summary) Summary {
	bound := func(text string, limit int) string {
		rs := []rune(text)
		for {
			raw, _ := json.Marshal(string(rs))
			if len(raw) <= limit {
				return string(rs)
			}
			rs = rs[:len(rs)-1]
		}
	}
	s.Title = bound(s.Title, 256)
	s.GoalSummary = bound(s.GoalSummary, 512)
	s.ProgressSummary = bound(s.ProgressSummary, 192)
	return s
}
