package web

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/z-chenhao/assistant/internal/agent"
)

type streamAccess struct {
	Core
	revoked atomic.Bool
}

func (c *streamAccess) CheckChannel(context.Context, string, string, string) error {
	if c.revoked.Load() {
		return errors.New("binding revoked")
	}
	return nil
}

func TestReplyStreamScopingAndRevocation(t *testing.T) {
	access := &streamAccess{}
	s := &Server{Core: access, Config: Config{TokenRef: "credential:test"}, Secrets: fakeSecrets{}}
	host := httptest.NewServer(s)
	defer host.Close()
	client := &http.Client{Timeout: 5 * time.Second}
	const path = "/api/conversation/stream"
	resp, err := client.Get(host.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatal(resp.Status)
	}
	emit := func(role agent.Role, generation, kind, text string) {
		s.ObserveReply(agent.Event{RunKind: string(role), GenerationID: generation, Kind: kind, Text: text})
	}
	emit(agent.MainRole, "main-generation", "message_start", "")
	emit(agent.MainRole, "main-generation", "message_update", "Visible")
	emit(agent.WorkerRole, "worker-generation", "message_start", "")
	emit(agent.WorkerRole, "worker-generation", "message_update", "Private worker")
	emit(agent.MainRole, "old-generation", "message_discarded", "")
	req, err := http.NewRequest(http.MethodGet, host.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer private-token-at-least-24-bytes")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatal(resp.Status, resp.Header)
	}
	reader := bufio.NewReader(resp.Body)
	next := func() replyFrame {
		t.Helper()
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if data, ok := strings.CutPrefix(line, "data: "); ok {
				var frame replyFrame
				if err := json.Unmarshal([]byte(data), &frame); err != nil {
					t.Fatal(err)
				}
				return frame
			}
		}
	}
	first := next()
	if first.Key != replyKey("main-generation") || first.State != "draft" || first.Text != "Visible" || !first.Reset {
		t.Fatal(first)
	}
	emit(agent.MainRole, "main-generation", "message_update", " suffix")
	if second := next(); second.Key != first.Key || second.State != "draft" || second.Text != " suffix" || second.Reset {
		t.Fatal(second)
	}
	access.revoked.Store(true)
	if tail, err := io.ReadAll(reader); err != nil || string(tail) != "\n" {
		t.Fatal("an idle SSE stream did not close cleanly after binding revocation:", string(tail), err)
	}
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatal(resp.Status)
	}
}

func TestReplyStreamCommitDeliversUnreadTailOnlyToExistingReader(t *testing.T) {
	s := &Server{}
	emit := func(generation, kind, text string) {
		s.ObserveReply(agent.Event{RunKind: string(agent.MainRole), GenerationID: generation, Kind: kind, Text: text})
	}
	emit("one", "message_start", "")
	emit("one", "message_update", "First")
	var original replyCursor
	if frame, _ := s.replies.read(&original); frame.Text != "First" || !frame.Reset {
		t.Fatal(frame)
	}
	emit("one", "message_update", " 中文")
	var reconnect replyCursor
	if frame, _ := s.replies.read(&reconnect); frame.Text != "First 中文" || !frame.Reset {
		t.Fatal(frame)
	}
	// Both an unread delta and a partial marker arrive before the next read.
	emit("one", "message_update", " tail ```stew")
	emit("one", "reply_committed", "")
	if frame, _ := s.replies.read(&original); frame.State != "committed" || frame.Text != " 中文 tail ```stew" {
		t.Fatal(frame)
	}
	if frame, _ := s.replies.read(&reconnect); frame.State != "committed" || frame.Text != " tail ```stew" {
		t.Fatal(frame)
	}
	if frame, _ := s.replies.read(&reconnect); frame.State != "" {
		t.Fatal("terminal frame replayed on same connection", frame)
	}
	var late replyCursor
	if frame, _ := s.replies.read(&late); frame.State != "committed" || frame.Text != "" {
		t.Fatal("new reader should load the committed projection", frame)
	}

	emit("two", "message_start", "")
	emit("two", "message_update", "Unfinished")
	emit("two", "message_discarded", "")
	emit("two", "message_update", "Late")
	if frame, _ := s.replies.read(&late); frame.State != "discarded" || frame.Text != "" {
		t.Fatal("cancelled text returned to the stream", frame)
	}
}
