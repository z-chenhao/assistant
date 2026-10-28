package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/z-chenhao/erii/internal/observability"
)

// ExecutionReader is separate from the conversation/action Core. Inspection
// reads remain available without a model runtime or a new execution pipeline.
type ExecutionReader interface {
	Executions(context.Context, observability.ExecutionQuery) (observability.Executions, error)
	Relationships(context.Context, observability.RelationshipQuery) (observability.Relationships, error)
}

type inspectionView struct {
	Query                          observability.ExecutionQuery
	RunID, TaskRef, RelationsAfter string
	List                           observability.Executions
	Relations                      *observability.Relationships
}

func inspectionQuery(r *http.Request) (*inspectionView, error) {
	v := &inspectionView{}
	q := r.URL.Query()
	for key, values := range q {
		switch key {
		case "role", "status", "since", "until", "cursor", "run", "task", "relations_after", "limit":
		default:
			return nil, observability.ErrInvalid
		}
		if len(values) != 1 {
			return nil, observability.ErrInvalid
		}
	}
	v.Query.Role = q.Get("role")
	v.Query.Status = q.Get("status")
	v.Query.Cursor = q.Get("cursor")
	var err error
	if value := q.Get("limit"); value != "" {
		v.Query.Limit, err = strconv.Atoi(value)
		if err != nil || v.Query.Limit < 1 {
			return nil, observability.ErrInvalid
		}
	}
	if v.Query.Since, err = inspectionTime(q.Get("since")); err != nil {
		return nil, err
	}
	if v.Query.Until, err = inspectionTime(q.Get("until")); err != nil {
		return nil, err
	}
	if err = v.Query.Validate(); err != nil {
		return nil, err
	}
	v.RunID = q.Get("run")
	v.TaskRef = q.Get("task")
	v.RelationsAfter = q.Get("relations_after")
	if v.RunID != "" || v.TaskRef != "" {
		err = (observability.RelationshipQuery{RunID: v.RunID, TaskRef: v.TaskRef, Cursor: v.RelationsAfter, Limit: v.Query.Limit}).Validate()
	} else if v.RelationsAfter != "" {
		err = observability.ErrInvalid
	}
	return v, err
}

func inspectionTime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	for _, format := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02T15:04"} {
		if at, err := time.Parse(format, raw); err == nil {
			return at.UTC(), nil
		}
	}
	return time.Time{}, observability.ErrInvalid
}
func timeInput(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format("2006-01-02T15:04:05")
}

func (s *Server) inspect(ctx context.Context, v *inspectionView, relationshipsOnly bool) error {
	if s.Inspector == nil {
		return errors.New("inspection unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if v.RunID != "" || v.TaskRef != "" {
		relations, err := s.Inspector.Relationships(ctx, observability.RelationshipQuery{RunID: v.RunID, TaskRef: v.TaskRef, Cursor: v.RelationsAfter, Limit: v.Query.Limit})
		if err != nil {
			return err
		}
		v.Relations = &relations
	} else if relationshipsOnly {
		return observability.ErrInvalid
	}
	if !relationshipsOnly {
		list, err := s.Inspector.Executions(ctx, v.Query)
		if err != nil {
			return err
		}
		v.List = list
	}
	return nil
}

func inspectionFailure(w http.ResponseWriter, err error) {
	status := 503
	if errors.Is(err, observability.ErrInvalid) {
		status = 400
	} else if errors.Is(err, observability.ErrNotFound) {
		status = 404
	}
	fail(w, status)
}

func (s *Server) inspectionAPI(w http.ResponseWriter, r *http.Request, relationships bool) {
	v, err := inspectionQuery(r)
	if err == nil && !relationships && (v.RunID != "" || v.TaskRef != "" || v.RelationsAfter != "") {
		err = observability.ErrInvalid
	}
	if err == nil && relationships && (v.Query.Role != "" || v.Query.Status != "" || !v.Query.Since.IsZero() || !v.Query.Until.IsZero() || v.Query.Cursor != "") {
		err = observability.ErrInvalid
	}
	if err == nil {
		err = s.inspect(r.Context(), v, relationships)
	}
	if err != nil {
		inspectionFailure(w, err)
		return
	}
	if relationships {
		jsonResult(w, v.Relations)
	} else {
		jsonResult(w, v.List)
	}
}

func inspectionURL(v *inspectionView, field, value string) string {
	q := url.Values{}
	if v.Query.Role != "" {
		q.Set("role", v.Query.Role)
	}
	if v.Query.Status != "" {
		q.Set("status", v.Query.Status)
	}
	if !v.Query.Since.IsZero() {
		q.Set("since", v.Query.Since.UTC().Format(time.RFC3339Nano))
	}
	if !v.Query.Until.IsZero() {
		q.Set("until", v.Query.Until.UTC().Format(time.RFC3339Nano))
	}
	if v.Query.Cursor != "" {
		q.Set("cursor", v.Query.Cursor)
	}
	if v.Query.Limit != 0 {
		q.Set("limit", strconv.Itoa(v.Query.Limit))
	}
	if v.RunID != "" {
		q.Set("run", v.RunID)
	}
	if v.TaskRef != "" {
		q.Set("task", v.TaskRef)
	}
	if v.RelationsAfter != "" {
		q.Set("relations_after", v.RelationsAfter)
	}
	switch field {
	case "run", "task":
		q.Del("run")
		q.Del("task")
		q.Del("relations_after")
		q.Set(field, value)
	case "list":
		q.Del("run")
		q.Del("task")
		q.Del("relations_after")
	case "cursor", "relations_after":
		q.Set(field, value)
	}
	if len(q) == 0 {
		return "/developer/observability"
	}
	return "/developer/observability?" + q.Encode()
}

func runRole(role string) string {
	switch role {
	case "main":
		return "Main"
	case "root":
		return "Root Worker"
	case "child":
		return "Child Worker"
	}
	return "Execution"
}
func taskAction(operation string) string {
	switch operation {
	case "create":
		return "Created Task"
	case "append":
		return "Appended instruction"
	case "pause":
		return "Paused Task"
	case "cancel":
		return "Cancelled Task"
	case "complete":
		return "Completed Task"
	}
	return "Recorded Task action"
}

type inspectionTreeNode struct {
	observability.Execution
	DisplayDepth  int
	ParentOutside bool
}

// Chronological pagination is not tree order: parallel siblings can create
// descendants later. Group only the nodes on this page by their actual parent;
// an absent parent starts an explicitly marked fragment, never a guessed edge.
func inspectionTree(items []observability.Execution) []inspectionTreeNode {
	present := map[string]bool{}
	children := map[string][]observability.Execution{}
	for _, v := range items {
		present[v.ID] = true
		children[v.Parent] = append(children[v.Parent], v)
	}
	out := []inspectionTreeNode{}
	seen := map[string]bool{}
	var visit func(observability.Execution, int)
	visit = func(v observability.Execution, depth int) {
		if seen[v.ID] {
			return
		}
		seen[v.ID] = true
		out = append(out, inspectionTreeNode{Execution: v, DisplayDepth: depth, ParentOutside: v.Parent != "" && !present[v.Parent]})
		for _, child := range children[v.ID] {
			visit(child, depth+1)
		}
	}
	for _, v := range items {
		if v.Parent == "" || !present[v.Parent] {
			visit(v, 0)
		}
	}
	// Corrupt metadata must not make rendering recurse forever or hide rows.
	for _, v := range items {
		visit(v, 0)
	}
	return out
}

func treeIndent(depth int) []struct{} { return make([]struct{}, max(0, min(depth, 16))) }
