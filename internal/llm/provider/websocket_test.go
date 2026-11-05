package provider

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func websocketUpgrade(t *testing.T, w http.ResponseWriter, r *http.Request) net.Conn {
	t.Helper()
	conn, buffer, err := w.(http.Hijacker).Hijack()
	if err != nil {
		t.Error(err)
		return nil
	}
	accept := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	fmt.Fprintf(buffer, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(accept[:]))
	buffer.Flush()
	return conn
}
func readClientFrame(conn net.Conn) (map[string]any, error) {
	h := make([]byte, 2)
	if _, err := io.ReadFull(conn, h); err != nil {
		return nil, err
	}
	if h[1]&128 == 0 {
		return nil, ErrInvalidResponse
	}
	size := int(h[1] & 127)
	if size == 126 {
		b := make([]byte, 2)
		if _, err := io.ReadFull(conn, b); err != nil {
			return nil, err
		}
		size = int(binary.BigEndian.Uint16(b))
	} else if size == 127 {
		return nil, ErrInvalidResponse
	}
	mask := make([]byte, 4)
	if _, err := io.ReadFull(conn, mask); err != nil {
		return nil, err
	}
	b := make([]byte, size)
	if _, err := io.ReadFull(conn, b); err != nil {
		return nil, err
	}
	for i := range b {
		b[i] ^= mask[i%4]
	}
	var value map[string]any
	if json.Unmarshal(b, &value) != nil {
		return nil, ErrInvalidResponse
	}
	return value, nil
}
func serverFrame(conn net.Conn, v any) error {
	b, _ := json.Marshal(v)
	frame := []byte{0x81}
	if len(b) < 126 {
		frame = append(frame, byte(len(b)))
	} else {
		frame = append(frame, 126)
		frame = binary.BigEndian.AppendUint16(frame, uint16(len(b)))
	}
	_, err := conn.Write(append(frame, b...))
	return err
}
func TestWebsocketCachedContinuationAndClose(t *testing.T) {
	var connections atomic.Int64
	observed := make(chan map[string]any, 2)
	output := []any{map[string]any{"type": "message", "role": "assistant", "id": "msg1", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": "hello", "annotations": []any{}}}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connections.Add(1)
		conn := websocketUpgrade(t, w, r)
		if conn == nil {
			return
		}
		defer conn.Close()
		for range 2 {
			v, err := readClientFrame(conn)
			if err != nil {
				return
			}
			observed <- v
			serverFrame(conn, map[string]any{"type": "response.output_text.delta", "delta": "hello"})
			serverFrame(conn, map[string]any{"type": "response.output_item.done", "output_index": 0, "item": output[0]})
			serverFrame(conn, map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp1", "status": "completed", "output": output, "usage": map[string]any{"input_tokens": 2, "output_tokens": 1}}})
		}
		io.Copy(io.Discard, conn)
	}))
	defer server.Close()
	client := &Client{}
	defer client.Close()
	model := Model{ID: "m", API: OpenAIResponses, BaseURL: server.URL}
	input := Context{Messages: []Message{{Role: "user", Content: []Content{Text("hi")}}}}
	o := Options{SessionID: "session", Transport: "websocket-cached", Timeout: time.Second}
	result, err := client.Complete(context.Background(), model, input, o)
	if err != nil {
		t.Fatal(err)
	}
	first := <-observed
	if first["type"] != "response.create" || first["previous_response_id"] != nil {
		t.Fatal(first)
	}
	input.Messages = append(input.Messages, Message{Role: "assistant", Content: result.Content}, Message{Role: "user", Content: []Content{Text("next")}})
	result, err = client.Complete(context.Background(), model, input, o)
	if err != nil {
		t.Fatal(err)
	}
	second := <-observed
	if second["previous_response_id"] != "resp1" || len(list(second["input"])) != 1 || connections.Load() != 1 {
		t.Fatal(second, connections.Load())
	}
	client.CloseSession("session")
}
func TestWebsocketCancellationAndFallback(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "websocket" {
			fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
			return
		}
		conn := websocketUpgrade(t, w, r)
		if conn == nil {
			return
		}
		defer conn.Close()
		if _, err := readClientFrame(conn); err == nil {
			close(started)
			io.Copy(io.Discard, conn)
		}
	}))
	defer server.Close()
	client := &Client{}
	defer client.Close()
	model := Model{ID: "m", API: CodexResponses, BaseURL: server.URL}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.Complete(ctx, model, Context{}, Options{Transport: "websocket", SessionID: "cancel"})
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if ErrorCode(err) != "GENERATION_CANCELLED" {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("websocket cancellation blocked")
	}
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") == "websocket" {
			w.WriteHeader(426)
			return
		}
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	}))
	defer fallback.Close()
	model.BaseURL = fallback.URL
	if _, err := client.Complete(context.Background(), model, Context{}, Options{Transport: "auto", Timeout: time.Second}); err != nil {
		t.Fatal(err)
	}
}
