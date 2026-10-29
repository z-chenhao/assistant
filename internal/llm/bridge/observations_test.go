package bridge

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/z-chenhao/erii/internal/llm"
)

func TestDispatchEvidenceIsOptionalBoundedAndCannotRejectGeneration(t *testing.T) {
	s := newEventStream("generation", 1024)
	count := 0
	s.wireSecrets = []string{"PRIVATE_KEY"}
	s.capture = func(d llm.Dispatch) {
		count++
		if bytes.Contains(d.Body, []byte("PRIVATE_KEY")) {
			t.Fatal("Host stored peer credential")
		}
	}
	raw := func(id, body string) json.RawMessage {
		b, _ := json.Marshal(llm.Dispatch{GenerationID: id, Attempt: "attempt", Phase: "started", At: 1, Body: json.RawMessage(body)})
		return b
	}
	s.observe(raw("other", `{}`))
	s.observe([]byte(`"malformed"`))
	if count != 0 {
		t.Fatal("cross-registration evidence accepted")
	}
	s.observe(raw("generation", `{"messages":[{"text":"PRIVATE_KEY"}],"authorization":"PRIVATE_KEY"}`))
	if count != 1 {
		t.Fatal("valid capture lost")
	}
	for range 30 {
		s.observe(raw("generation", `{}`))
	}
	if count != 16 {
		t.Fatal("unbounded diagnostics", count)
	}
	s.capture = func(llm.Dispatch) { panic("diagnostic sink failure") }
	s.observed = 0
	s.observe(raw("generation", `{}`))
	s.accept(llm.GenerationEvent{GenerationID: s.id, Sequence: 1, Kind: "text_delta", Text: "model still works"})
	if s.terminal || (<-s.events).event.Text != "model still works" {
		t.Fatal("diagnostics changed generation event sequence")
	}
}
