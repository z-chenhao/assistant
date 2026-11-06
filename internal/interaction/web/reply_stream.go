package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/z-chenhao/assistant/internal/agent"
)

// A workspace has one active Main. Keep only its current preview and latest
// disposition. Readers take suffixes independently; a slow tab cannot queue
// events or block the actor on network I/O.
type replyPreview struct {
	mu         sync.Mutex
	changed    chan struct{}
	revision   uint64
	generation string
	key        string
	state      string
	text       strings.Builder
	cut        bool
}

const previewLimit = 64 << 10

func replyKey(generation string) string {
	sum := sha256.Sum256([]byte(generation))
	return hex.EncodeToString(sum[:16])
}

// ObserveReply consumes in-process Agent events. The browser protocol never
// carries Run IDs, raw tool payloads, reasoning or internal lifecycle events.
func (s *Server) ObserveReply(e agent.Event) {
	if strings.HasPrefix(e.Kind, "ui_preview_") {
		s.observeUIPreview(e)
		return
	}
	if e.RunKind != string(agent.MainRole) || e.GenerationID == "" {
		return
	}
	p := &s.replies
	p.mu.Lock()
	defer p.mu.Unlock()
	switch e.Kind {
	case "message_start":
		p.generation, p.key, p.state = e.GenerationID, replyKey(e.GenerationID), "draft"
		p.text.Reset()
		p.cut = false
		p.revision++
	case "message_update":
		if p.state != "draft" || p.generation != e.GenerationID || !p.appendText(e.Text) {
			return
		}
	case "message_discarded", "reply_committed":
		if p.state != "draft" || p.generation != e.GenerationID {
			return
		}
		p.state = "discarded"
		if e.Kind == "reply_committed" {
			p.state = "committed"

		} else {
			p.text.Reset()
		}
		p.cut = false
		p.revision++
	default:
		return
	}
	if p.changed != nil {
		close(p.changed)
		p.changed = nil
	}
}

func (p *replyPreview) appendText(delta string) bool {
	if delta == "" || p.cut {
		return false
	}
	text := delta
	if remaining := previewLimit - p.text.Len(); len(text) > remaining {
		for remaining > 0 && !utf8.RuneStart(text[remaining]) {
			remaining--
		}
		text, p.cut = text[:remaining], true
	}
	p.text.WriteString(text)
	return text != ""
}

type replyFrame struct {
	Key   string `json:"key,omitempty"`
	State string `json:"state"`
	Text  string `json:"text,omitempty"`
	Reset bool   `json:"reset,omitempty"`
}

type replyCursor struct {
	revision uint64
	offset   int
}

func (p *replyPreview) read(cursor *replyCursor) (replyFrame, <-chan struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.changed == nil {
		p.changed = make(chan struct{})
	}
	reset := cursor.revision != p.revision
	if !reset && cursor.offset == p.text.Len() {
		return replyFrame{}, p.changed
	}
	frame := replyFrame{Key: p.key, State: p.state, Reset: reset}
	if reset {
		// Only an existing reader of this draft needs the tail on commit.
		// A new connection loads finished messages through Conversation.
		if p.state == "committed" && cursor.revision != 0 && cursor.revision+1 == p.revision {
			frame.Text = p.text.String()[cursor.offset:]
		}
		cursor.revision, cursor.offset = p.revision, 0
	}
	if p.state == "draft" {
		frame.Text = p.text.String()[cursor.offset:]
	}
	cursor.offset = p.text.Len()
	return frame, p.changed
}

func (s *Server) streamReplies(w http.ResponseWriter, r *http.Request) {
	// ServeHTTP has already applied loopback, token and binding checks. Renew
	// that read authorization during long connections, even when idle.
	ctx := r.Context()
	var shutdown <-chan struct{}
	if s.lifetime != nil {
		shutdown = s.lifetime.Done()
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Vary", "Cookie, Authorization")
	control := http.NewResponseController(w)
	write := func(frame string) bool {
		_ = control.SetWriteDeadline(time.Now().Add(3 * time.Second))
		_, err := io.WriteString(w, frame)
		if err != nil || control.Flush() != nil {
			return false
		}
		_ = control.SetWriteDeadline(time.Time{}) // Only bound writes, not idle time.
		return true
	}
	if !write("retry: 2000\n\n") {
		return
	}
	nextCheck := time.Now().Add(time.Second)
	auth := time.NewTimer(time.Until(nextCheck))
	defer auth.Stop()
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	var cursor replyCursor
	var uiVersion uint64
	// Bound flush frequency without adding a delay to the first chunk.
	batch := time.NewTicker(25 * time.Millisecond)
	defer batch.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-shutdown:
			return
		default:
		}
		if !time.Now().Before(nextCheck) {
			check, cancel := context.WithTimeout(ctx, time.Second)
			allowed := s.authorized(r.WithContext(check)) && s.Core.CheckChannel(check, "web", Subject, Destination) == nil
			cancel()
			if !allowed {
				return
			}
			nextCheck = time.Now().Add(time.Second)
			auth.Reset(time.Until(nextCheck))
		}
		uiFrame, uiChanged := s.uiPreviews.read(&uiVersion)
		if uiFrame.State != "" {
			data, err := json.Marshal(uiFrame)
			if err != nil || !write("event: ui-preview\ndata: "+string(data)+"\n\n") {
				return
			}
		}
		frame, changed := s.replies.read(&cursor)
		if frame.State == "" && uiFrame.State != "" {
			select {
			case <-ctx.Done():
				return
			case <-shutdown:
				return
			case <-batch.C:
			}
			continue
		}
		if frame.State != "" {
			data, err := json.Marshal(frame)
			if err != nil || !write("event: reply\ndata: "+string(data)+"\n\n") {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-shutdown:
				return
			case <-batch.C:
			}
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-shutdown:
			return
		case <-changed:
		case <-uiChanged:
		case <-auth.C:
		case <-heartbeat.C:
			if !write(": keepalive\n\n") {
				return
			}
		}
	}
}
