package secret

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
)

// TestSocket is explicit CI injection into an owned, in-memory Unix-socket
// fixture. It never persists secrets and is never selected by configuration.
type TestSocket struct{ Path string }

func (s TestSocket) call(ctx context.Context, op, ref string, value []byte) ([]byte, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", s.Path)
	if err != nil {
		return nil, llm.ErrSecretStore
	}
	defer conn.Close()
	deadline := time.Now().Add(5 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	request := struct {
		Op    string `json:"op"`
		Ref   string `json:"ref"`
		Value []byte `json:"value,omitempty"`
	}{op, ref, value}
	if json.NewEncoder(conn).Encode(request) != nil {
		return nil, llm.ErrSecretStore
	}
	var response struct {
		OK    bool   `json:"ok"`
		Value []byte `json:"value"`
	}
	if json.NewDecoder(io.LimitReader(conn, 256<<10)).Decode(&response) != nil || !response.OK {
		return nil, llm.ErrSecretStore
	}
	return response.Value, nil
}
func (s TestSocket) Get(ctx context.Context, ref string) ([]byte, error) {
	return s.call(ctx, "get", ref, nil)
}
func (s TestSocket) Put(ctx context.Context, ref string, value []byte) error {
	_, err := s.call(ctx, "put", ref, value)
	return err
}
func (s TestSocket) Delete(ctx context.Context, ref string) error {
	_, err := s.call(ctx, "delete", ref, nil)
	return err
}
