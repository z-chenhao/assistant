package bridge

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/z-chenhao/collie/internal/llm"
)

func TestMailboxClipsBeforeRetainingPayload(t *testing.T) {
	s := newEventStream("generation", 1024)
	text := strings.Repeat("界", 3000)
	for i := int64(1); i <= 128; i++ {
		s.accept(llm.GenerationEvent{GenerationID: "generation", Sequence: i, Kind: "text_delta", Text: text,
			ToolCall: &llm.ToolCall{Arguments: text}, ErrorCode: text})
	}
	s.accept(llm.GenerationEvent{GenerationID: "generation", Sequence: 129, Kind: "completed", Text: text})
	bytes, count, completions := 0, 0, 0
	clipped := false
	for len(s.events) != 0 {
		d := <-s.events
		count++
		if d.err != nil || !utf8.ValidString(d.event.Text) || d.event.ToolCall != nil || d.event.ErrorCode != "" {
			t.Fatal("queued invalid or inapplicable payload", d.err)
		}
		bytes += len(d.event.Text)
		clipped = clipped || d.clipped
		if d.event.Kind == "completed" {
			completions++
		}
	}
	if count != 129 || completions != 1 || bytes != 4095 || !clipped {
		t.Fatal("mailbox retained output beyond the UTF-8 byte budget or lost events", count, completions, bytes, clipped)
	}
}

func TestMailboxRejectsChangedDuplicateAfterClipping(t *testing.T) {
	s := newEventStream("generation", 1)
	e := llm.GenerationEvent{GenerationID: "generation", Sequence: 1, Kind: "text_delta", Text: "same-prefix-original"}
	s.accept(e)
	s.accept(e) // a matching duplicate occupies no additional mailbox slot
	e.Text = "same-prefix-changed"
	s.accept(e) // clipping both to "same" must not hide drift
	if len(s.events) != 2 {
		t.Fatal("duplicate delivery count", len(s.events))
	}
	if d := <-s.events; d.event.Text != "same" || !d.clipped {
		t.Fatal(d)
	}
	if d := <-s.events; !errors.Is(d.err, llm.InvalidProviderResponse) {
		t.Fatal("changed duplicate survived normalization", d.err)
	}
}
