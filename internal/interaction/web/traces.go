package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/observability"
)

type TraceReader interface {
	Traces(context.Context, observability.TraceQuery) (observability.Traces, error)
	Observations(context.Context, observability.ObservationQuery) (observability.ObservationTree, error)
	ObservationBody(context.Context, observability.BodyQuery) (observability.Body, error)
	ChatPreview(context.Context, observability.ObservationQuery) ([]observability.ChatPreview, error)
}

type traceView struct {
	Query                              observability.TraceQuery
	Run, Task, Node, After, Tab, Field string
	Offset                             int
	DiffAfter                          int
	List                               observability.Traces
	Tree                               *observability.ObservationTree
	Rows                               []traceRow
	Input, Output, Wire                observability.Body
	Messages                           []observability.ChatPreview
	Preparation                        *observability.FrozenContext
	Schemas                            *observability.FrozenContext
	ModelOutput                        *llm.GenerationResult
	Diff                               *observability.ContextDiff
	Relations                          *observability.Relationships
}
type traceRow struct {
	observability.Observation
	Depth         int
	ParentOutside bool
	X, Width      float64
}

func (s *Server) loadTrace(ctx context.Context, v *traceView) error {
	if s.Traces == nil {
		return errors.New("trace inspection unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var err error
	v.List, err = s.Traces.Traces(ctx, v.Query)
	if err != nil {
		return err
	}
	if v.Task != "" || v.Run != "" {
		if s.Inspector == nil {
			return errors.New("relationships unavailable")
		}
		relations, e := s.Inspector.Relationships(ctx, observability.RelationshipQuery{RunID: v.Run, TaskRef: v.Task})
		if e != nil {
			return e
		}
		v.Relations = &relations
	}
	if v.Run == "" {
		return nil
	}
	query := observability.ObservationQuery{RunID: v.Run, Selected: v.Node, Cursor: v.After}
	tree, err := s.Traces.Observations(ctx, query)
	if err != nil {
		return err
	}
	v.Tree = &tree
	v.Node = tree.Selected.ID
	query.Selected = v.Node
	v.Rows = traceRows(tree)
	read := func(field string) (observability.Body, error) {
		offset := 0
		if field == v.Field {
			offset = v.Offset
		}
		return s.Traces.ObservationBody(ctx, observability.BodyQuery{ObservationQuery: query, Field: field, Offset: offset})
	}
	if v.Tab == "wire" {
		v.Wire, err = read("wire")
		return err
	}
	if v.Tab == "metadata" {
		return nil
	}
	if tree.Selected.Type == "generation" {
		if v.Tab == "diff" {
			if s.Contexts == nil {
				return errors.New("context unavailable")
			}
			diff, e := s.Contexts.ContextDiff(ctx, observability.DiffQuery{RunID: tree.Selected.RunID, Step: *tree.Selected.Step, Section: "messages", After: v.DiffAfter})
			v.Diff = &diff
			return e
		}
		if v.Tab == "context" && s.Contexts != nil {
			prep, e := s.Contexts.Context(ctx, observability.ContextQuery{RunID: tree.Selected.RunID, Step: *tree.Selected.Step, Section: "provenance"})
			if e != nil {
				return e
			}
			v.Preparation = &prep
			schemas, e := s.Contexts.Context(ctx, observability.ContextQuery{RunID: tree.Selected.RunID, Step: *tree.Selected.Step, Section: "tools", JSON: true})
			if e != nil {
				return e
			}
			v.Schemas = &schemas
		}
		v.Messages, err = s.Traces.ChatPreview(ctx, query)
		if err != nil {
			return err
		}
		v.Output, err = read("output")
		if err == nil && v.Output.Available && v.Offset == 0 && v.Output.Next == nil {
			var out llm.GenerationResult
			if json.Unmarshal([]byte(v.Output.Content), &out) == nil {
				v.ModelOutput = &out
			}
		}
		return err
	}
	v.Input, err = read("input")
	if err != nil {
		return err
	}
	v.Output, err = read("output")
	return err
}

func (s *Server) traceAPI(w http.ResponseWriter, r *http.Request, kind string) {
	v, err := traceQuery(r)
	if err == nil && s.Traces == nil {
		err = errors.New("trace unavailable")
	}
	if err != nil {
		inspectionFailure(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	q := observability.ObservationQuery{RunID: v.Run, Selected: v.Node, Cursor: v.After}
	if kind != "traces" {
		err = q.Validate()
	}
	if err == nil && kind == "body" {
		err = (observability.BodyQuery{ObservationQuery: q, Field: v.Field, Offset: v.Offset}).Validate()
	}
	if err != nil {
		inspectionFailure(w, err)
		return
	}
	var result any
	switch kind {
	case "traces":
		result, err = s.Traces.Traces(ctx, v.Query)
	case "observations":
		result, err = s.Traces.Observations(ctx, q)
	case "body":
		result, err = s.Traces.ObservationBody(ctx, observability.BodyQuery{ObservationQuery: q, Field: v.Field, Offset: v.Offset})
	}
	if err != nil {
		inspectionFailure(w, err)
		return
	}
	jsonResult(w, result)
}
