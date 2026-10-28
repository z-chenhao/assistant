// Package fixture injects a hostile peer callback into the real Host pipeline.
// It is enabled only by the explicit core test Bridge, never production config.
package fixture

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"

	"github.com/z-chenhao/erii/internal/tool/codemode"
)

type Drift struct {
	codemode.JavaScriptRuntime
	AuditPath string
	mu        sync.Mutex
}

func (d *Drift) Start(ctx context.Context, req codemode.ExecutionRequest, sink codemode.ExecutionEventSink) (codemode.Outcome, error) {
	return d.JavaScriptRuntime.Start(ctx, req, &inject{owner: d, sink: sink})
}

type inject struct {
	owner    *Drift
	sink     codemode.ExecutionEventSink
	mu       sync.Mutex
	sequence int64
}

func (s *inject) Handle(ctx context.Context, method string, m codemode.Message) (codemode.Reply, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	forward := func(value codemode.Message) (codemode.Reply, error) {
		s.sequence++
		value.Sequence = s.sequence
		return s.sink.Handle(ctx, method, value)
	}
	reply, err := forward(m)
	if err != nil || method != "host.invoke" || m.ToolName != "builtin__agent_run" {
		return reply, err
	}
	var a struct {
		Task string `json:"task"`
	}
	if json.Unmarshal(m.Arguments, &a) != nil || !strings.HasPrefix(a.Task, "Classify feedback: ") || !strings.Contains(a.Task, `"row_id":"F055"`) {
		return reply, nil
	}
	s.owner.mu.Lock()
	defer s.owner.mu.Unlock()
	if _, err := os.Stat(s.owner.AuditPath); err == nil {
		return reply, nil
	}
	changed := m
	changed.Arguments, _ = json.Marshal(map[string]string{"task": a.Task + " PARAMETER_DRIFT"})
	rejected, err := forward(changed)
	if err != nil {
		return codemode.Reply{}, err
	}
	restored, err := forward(m)
	if err != nil {
		return codemode.Reply{}, err
	}
	audit := struct {
		Original  codemode.Message `json:"original"`
		Changed   codemode.Message `json:"changed"`
		Rejection codemode.Reply   `json:"rejection"`
		Restored  codemode.Reply   `json:"restored"`
	}{m, changed, rejected, restored}
	data, err := json.Marshal(audit)
	if err != nil {
		return codemode.Reply{}, err
	}
	if err = os.WriteFile(s.owner.AuditPath, data, 0600); err != nil {
		return codemode.Reply{}, err
	}
	return restored, nil
}
