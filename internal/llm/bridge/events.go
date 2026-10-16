package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/z-chenhao/erii/internal/llm"
)

const maxGenerationEvents = 4096

type eventDelivery struct {
	event    llm.GenerationEvent
	clipped  bool
	err      error
	rejected bool // Host validation, rather than a provider terminal error
}

// The process mutex protects validation; settleGate protects final acceptance
// and the cancellation signal. Only normalized deliveries cross to the sink
// goroutine. The mailbox holds every allowed distinct event plus one rejection,
// so a slow sink never blocks the shared RPC reader.
type eventStream struct {
	events       chan eventDelivery
	id           string
	sessionID    string
	seen         map[int64][32]byte
	last         int64
	remaining    int
	toolCalls    int
	terminal     bool
	cancelled    chan struct{}
	dispatchDone chan struct{} // closed after start/continuation write, or failed admission
	interrupt    context.CancelFunc
	settleGate   chan struct{} // only usage commit/cancellation, never a sink/RPC
	settled      bool
}

func newEventStream(id string, maxOutputTokens int64) *eventStream {
	limit := MaxOutputBytes
	if maxOutputTokens < int64(limit/4) {
		limit = int(maxOutputTokens * 4)
	}
	s := &eventStream{events: make(chan eventDelivery, maxGenerationEvents+1), cancelled: make(chan struct{}), dispatchDone: make(chan struct{}), settleGate: make(chan struct{}, 1), id: id, seen: map[int64][32]byte{}, remaining: limit}
	s.settleGate <- struct{}{}
	return s
}

func (s *eventStream) reject(err error) {
	s.terminal = true
	s.events <- eventDelivery{err: err, rejected: true}
}

func (s *eventStream) wasCancelled() bool {
	select {
	case <-s.cancelled:
		return true
	default:
		return false
	}
}

// Called for the captured registration after a cancellation or disposal ack.
// A retired registration is permanently settled, so a late ack cannot affect a
// new registration reusing the ID. No process lock is held while awaiting commit.
func (s *eventStream) cancel(ctx context.Context) error {
	select {
	case <-s.settleGate:
	case <-ctx.Done():
		return llm.ProviderUnavailable
	}
	defer func() { s.settleGate <- struct{}{} }()
	if ctx.Err() != nil {
		return llm.ProviderUnavailable
	}
	if s.settled || s.wasCancelled() {
		return nil
	}
	close(s.cancelled)
	if s.interrupt != nil {
		s.interrupt()
	}
	return nil
}

// finalize is called by Host persistence after opening its transaction. The
// short commit and cancellation share one per-registration acceptance point.
func (s *eventStream) finalize(commit func(bool) error) error {
	<-s.settleGate
	defer func() { s.settleGate <- struct{}{} }()
	if s.settled {
		return llm.InvalidProviderResponse
	}
	err := commit(s.wasCancelled())
	s.settled = true
	return err
}

func (s *eventStream) accept(e llm.GenerationEvent) {
	if s.terminal || s.wasCancelled() {
		return
	}
	// Hash the original payload before clipping; changed duplicates still fail.
	encoded, _ := json.Marshal(e)
	digest := sha256.Sum256(encoded)
	if prior, ok := s.seen[e.Sequence]; ok {
		if prior != digest {
			s.reject(llm.InvalidProviderResponse)
		}
		return
	}
	if e.Sequence != s.last+1 || e.Sequence > maxGenerationEvents {
		s.reject(llm.InvalidProviderResponse)
		return
	}
	s.last = e.Sequence
	s.seen[s.last] = digest
	// Retain only the kind's payload and reuse the registered ID. Inapplicable
	// fields and clipped string backing storage cannot amplify queued memory.
	d := eventDelivery{event: llm.GenerationEvent{GenerationID: s.id, Sequence: e.Sequence, Kind: e.Kind}}
	switch e.Kind {
	case "text_delta":
		n := len(e.Text)
		if n > s.remaining {
			n = s.remaining
			for n > 0 && !utf8.ValidString(e.Text[:n]) {
				n--
			}
			d.clipped = true
		}
		d.event.Text = strings.Clone(e.Text[:n])
		s.remaining -= n
	case "tool_call":
		if e.ToolCall == nil || e.ToolCall.ID == "" || e.ToolCall.Name == "" || s.toolCalls >= 32 || len(e.ToolCall.Arguments) > MaxOutputBytes {
			s.reject(llm.InvalidProviderResponse)
			return
		}
		s.toolCalls++
		d.event.ToolCall = e.ToolCall
	case "completed":
		if e.Usage != nil {
			if e.Usage.InputTokens < 0 || e.Usage.OutputTokens < 0 || e.Usage.CacheReadTokens < 0 || e.Usage.CacheWriteTokens < 0 {
				s.reject(llm.InvalidProviderResponse)
				return
			}
			d.event.Usage = e.Usage
		}
		s.terminal = true
	case "error":
		s.terminal = true
		s.events <- eventDelivery{err: llm.NormalizeError(e.ErrorCode)}
		return
	default:
		s.reject(llm.InvalidProviderResponse)
		return
	}
	s.events <- d
}
