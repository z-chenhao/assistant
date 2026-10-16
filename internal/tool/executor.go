// Package tool owns the single ToolSpec -> ToolInvocation -> Executor path.
// Pure Tools and trusted internal Task controls are available. External effects
// remain closed until the M3 policy/approval pipeline.
package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
)

type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type Result struct {
	Text  string `json:"text"`
	Error *Error `json:"error,omitempty"`
}

func Failure(code string) Result {
	return Result{Error: &Error{Code: code, Message: code, Retryable: code == "PROVIDER_UNAVAILABLE"}}
}
func (r Result) ModelText() string {
	if r.Error == nil {
		return r.Text
	}
	b, _ := json.Marshal(r.Error)
	return string(b)
}

type Spec struct {
	Name         string `json:"name"`
	Version      int    `json:"version"`
	InputSchema  string `json:"input_schema"`
	ReplayPolicy string `json:"replay_policy"`
	Permission   string `json:"permission"`
}

type Definition struct {
	Spec    Spec
	Host    bool // Trusted Host builtins receive the accepted invocation, never model-supplied IDs.
	Execute func(context.Context, string) Result
}
type Registry struct{ definitions map[string]Definition }

func NewRegistry(definitions []Definition) (*Registry, error) {
	r := &Registry{definitions: map[string]Definition{}}
	for _, d := range definitions {
		// Until M3 policy/approval exists, external effects remain closed.
		// Task control is an internal transaction, bound to an accepted call.
		if d.Spec.Name == "" || d.Spec.Version != 1 || d.Spec.ReplayPolicy != "safe" || !validDefinition(d) || !json.Valid([]byte(d.Spec.InputSchema)) {
			return nil, errors.New("invalid tool definition")
		}
		if _, exists := r.definitions[d.Spec.Name]; exists {
			return nil, errors.New("duplicate tool definition")
		}
		r.definitions[d.Spec.Name] = d
	}
	return r, nil
}
func validDefinition(d Definition) bool {
	if d.Host {
		return d.Execute == nil && ((d.Spec.Name == "builtin__task__search" && d.Spec.Permission == "read_only") || (d.Spec.Name == "builtin__task__manage" && d.Spec.Permission == "task_control"))
	}
	return d.Execute != nil && d.Spec.Permission == "read_only"
}

// Select verifies every frozen spec before constructing the role-specific view.
func (r *Registry) Select(specs []Spec) (*Registry, error) {
	defs := make([]Definition, 0, len(specs))
	for _, s := range specs {
		d, ok := r.Resolve(s.Name)
		if !ok || !reflect.DeepEqual(d.Spec, s) {
			return nil, llm.ModelIncompatible
		}
		defs = append(defs, d)
	}
	return NewRegistry(defs)
}

// HostStore executes registered internal controls under the same accepted,
// fenced ToolInvocation. It is not an alternative invocation lifecycle.
type HostStore interface {
	ExecuteBuiltin(context.Context, Invocation) (Result, error)
}

func EchoRegistry() *Registry {
	r, err := NewRegistry([]Definition{{Spec: Spec{Name: "builtin__text__echo", Version: 1, InputSchema: `{"type":"object","properties":{"text":{"type":"string","maxLength":4096}},"required":["text"],"additionalProperties":false}`, ReplayPolicy: "safe", Permission: "read_only"}, Execute: func(ctx context.Context, args string) Result {
		var value struct {
			Text *string `json:"text"`
		}
		dec := json.NewDecoder(bytes.NewBufferString(args))
		dec.DisallowUnknownFields()
		if dec.Decode(&value) != nil || value.Text == nil || len(*value.Text) > 4096 {
			return Failure("INVALID_ARGUMENT")
		}
		var extra any
		if dec.Decode(&extra) != io.EOF {
			return Failure("INVALID_ARGUMENT")
		}
		return Result{Text: *value.Text}
	}}})
	if err != nil {
		panic(err)
	}
	return r
}
func (r *Registry) Describe() []Spec {
	// M1 has one Tool. Sorting keeps capability hashing stable as it grows.
	out := make([]Spec, 0, len(r.definitions))
	for _, d := range r.definitions {
		out = append(out, d.Spec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func (r *Registry) Resolve(name string) (Definition, bool) {
	d, ok := r.definitions[name]
	return d, ok
}

type Invocation struct {
	ID     string
	Call   llm.ToolCall
	Status string
	Result Result
}

// Store is bound by the Host to an opaque Run claim. Every mutation fences.
type Store interface {
	Accept(context.Context, string, []llm.ToolCall, []Spec) ([]Invocation, error)
	Start(context.Context, string) error
	Finish(context.Context, string, Result) error
}
type Event struct {
	Kind         string
	InvocationID string
	Call         llm.ToolCall
	Result       Result
}
type Executor struct {
	Registry    *Registry
	Store       Store
	Concurrency int
	Sink        func(Event)
}

func (e Executor) Execute(ctx context.Context, exchange string, calls []llm.ToolCall) ([]Result, error) {
	if len(calls) > 64 {
		return nil, errors.New("tool batch resource limit")
	}
	invocations, err := e.Store.Accept(ctx, exchange, calls, e.Registry.Describe())
	if err != nil {
		return nil, err
	}
	limit := e.Concurrency
	if limit < 1 {
		limit = 4
	}
	if limit > 16 {
		limit = 16
	}
	results := make([]Result, len(invocations))
	errs := make([]error, len(invocations))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for n := 0; n < limit; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				inv := invocations[i]
				if inv.Status == "succeeded" || inv.Status == "failed" {
					results[i] = inv.Result
					continue
				}
				if ctx.Err() != nil {
					errs[i] = ctx.Err()
					continue
				}
				_, known := e.Registry.Resolve(inv.Call.Name)
				// A missing Tool has no external execution to replay. A crash after
				// its rejection admission can still finish the same denied record.
				if inv.Status != "running" || known {
					if err := e.Store.Start(ctx, inv.ID); err != nil {
						errs[i] = err
						continue
					}
				}
				if e.Sink != nil {
					e.Sink(Event{Kind: "tool_execution_start", InvocationID: inv.ID, Call: inv.Call})
				}
				result := Failure("NOT_FOUND")
				if d, ok := e.Registry.Resolve(inv.Call.Name); ok {
					callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
					if d.Host {
						host, ok := e.Store.(HostStore)
						if !ok {
							result = Failure("PERMISSION_DENIED")
						} else {
							var hostErr error
							result, hostErr = host.ExecuteBuiltin(callCtx, inv)
							if hostErr != nil {
								cancel()
								errs[i] = hostErr
								continue
							}
						}
					} else {
						result = d.Execute(callCtx, inv.Call.Arguments)
					}
					if callCtx.Err() != nil && !d.Host {
						result = Failure("TIMEOUT")
					}
					cancel()
				}
				limitBytes := 16 << 10
				if inv.Call.Name == "builtin__task__search" {
					limitBytes = 64 << 10
				}
				if len(result.Text) > limitBytes {
					result = Failure("RESOURCE_LIMIT")
				}
				if err := e.Store.Finish(ctx, inv.ID, result); err != nil {
					errs[i] = err
					continue
				}
				results[i] = result
				if e.Sink != nil {
					e.Sink(Event{Kind: "tool_execution_end", InvocationID: inv.ID, Call: inv.Call, Result: result})
				}
			}
		}()
	}
	for i := range invocations {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}
