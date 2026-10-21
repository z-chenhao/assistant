package fixture

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/z-chenhao/erii/internal/tool"
	"github.com/z-chenhao/erii/internal/tool/codemode"
)

type ledger struct{ messages []codemode.Message }

func (l *ledger) Handle(_ context.Context, _ string, m codemode.Message) (codemode.Reply, error) {
	l.messages = append(l.messages, m)
	if len(l.messages) == 2 {
		return codemode.Reply{Error: tool.Failure("NON_DETERMINISTIC_REPLAY").Error}, nil
	}
	return codemode.Reply{Suspended: true}, nil
}
func TestHostileCallbackKeepsRuntimeKeyAndRestoresOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drift.json")
	d := &Drift{AuditPath: path}
	l := &ledger{}
	sink := &inject{owner: d, sink: l}
	args, _ := json.Marshal(map[string]string{"task": `Classify feedback: {"row_id":"F055","text":"希望增加深色模式"}`})
	m := codemode.Message{Sequence: 42, ExecutionID: "frozen", ClaimToken: "owner", ExecutionEpoch: 3, CallID: "55", CallKey: "runtime-generated", ToolName: "builtin__agent__run", Arguments: args}
	r, err := sink.Handle(context.Background(), "host.invoke", m)
	if err != nil || !r.Suspended || len(l.messages) != 3 {
		t.Fatalf("%+v %v", r, err)
	}
	for i, seen := range l.messages {
		if seen.Sequence != int64(i+1) || seen.CallKey != m.CallKey || seen.ExecutionID != m.ExecutionID || seen.ExecutionEpoch != 3 || seen.ClaimToken != m.ClaimToken {
			t.Fatal("injection changed trusted identity")
		}
	}
	if string(l.messages[0].Arguments) != string(l.messages[2].Arguments) || string(l.messages[1].Arguments) == string(args) {
		t.Fatal("did not restore frozen arguments")
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal(err)
	}
	// A new Host/runtime can replay the same key without reinjecting the fault.
	next := &inject{owner: &Drift{AuditPath: path}, sink: l}
	_, err = next.Handle(context.Background(), "host.invoke", m)
	if err != nil || len(l.messages) != 4 {
		t.Fatal("repeated injection after restart")
	}
}
