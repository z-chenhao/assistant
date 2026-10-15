// Package bridge owns the versioned, private stdio Provider RPC. It never
// exposes child stderr or provider response bodies as errors.
package bridge

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/z-chenhao/erii/internal/llm"
)

const ProtocolVersion = 1
const MaxFrameBytes = 1 << 20
const MaxOutputBytes = 64 << 10

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type frame struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type process struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	write   sync.Mutex
	mu      sync.Mutex
	pending map[uint64]chan frame
	events  map[string]chan llm.GenerationEvent
	done    chan struct{}
	wait    chan struct{}
	failure error
}

type Client struct {
	mu     sync.Mutex
	path   string
	args   []string
	p      *process
	closed bool
	next   atomic.Uint64
}

// Open takes a Host-selected executable and arguments, never model input.
// The child receives no inherited credentials, application configuration or DB
// paths. This is a process boundary, not an OS filesystem/network sandbox.
func Open(ctx context.Context, path string, args ...string) (*Client, error) {
	c := &Client{path: path, args: append([]string(nil), args...)}
	if _, err := c.ensure(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) ensure(ctx context.Context) (*process, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, llm.ProviderUnavailable
	}
	if c.p != nil {
		select {
		case <-c.p.done:
			c.stop(c.p)
			c.p = nil
		default:
			return c.p, nil
		}
	}
	cmd := exec.Command(c.path, c.args...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	cmd.Dir = os.TempDir()
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, llm.ProviderUnavailable
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		_ = in.Close()
		return nil, llm.ProviderUnavailable
	}
	if cmd.Start() != nil {
		_ = in.Close()
		_ = out.Close()
		return nil, llm.ProviderUnavailable
	}
	p := &process{cmd: cmd, in: in, pending: map[uint64]chan frame{}, events: map[string]chan llm.GenerationEvent{}, done: make(chan struct{}), wait: make(chan struct{})}
	go p.read(out)
	// Wait only after read has consumed stdout. Close always reaps this child.
	go func() { <-p.done; _ = cmd.Wait(); close(p.wait) }()
	var hello struct {
		ProtocolVersion int `json:"protocol_version"`
	}
	if err := c.callProcess(ctx, p, "bridge.initialize", map[string]int{"protocol_version": ProtocolVersion}, &hello); err != nil || hello.ProtocolVersion != ProtocolVersion {
		c.stop(p)
		return nil, llm.ModelIncompatible
	}
	c.p = p
	return p, nil
}

func (p *process) read(out io.Reader) {
	defer close(p.done)
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4096), MaxFrameBytes)
	for scanner.Scan() {
		var f frame
		if json.Unmarshal(scanner.Bytes(), &f) != nil || f.JSONRPC != "2.0" {
			p.fail(llm.InvalidProviderResponse)
			return
		}
		p.mu.Lock()
		if f.ID != 0 && f.Method == "" && (f.Error != nil || f.Result != nil) {
			if ch := p.pending[f.ID]; ch != nil {
				select {
				case ch <- f:
				default:
				}
			}
		} else if f.ID == 0 && f.Method == "generation.event" {
			var e llm.GenerationEvent
			if json.Unmarshal(f.Params, &e) != nil {
				p.mu.Unlock()
				p.fail(llm.InvalidProviderResponse)
				return
			}
			if ch := p.events[e.GenerationID]; ch != nil {
				select {
				case ch <- e:
				default:
					p.mu.Unlock()
					p.fail(llm.InvalidProviderResponse)
					return
				}
			}
		} else {
			p.mu.Unlock()
			p.fail(llm.InvalidProviderResponse)
			return
		}
		p.mu.Unlock()
	}
	if scanner.Err() != nil {
		p.fail(llm.InvalidProviderResponse)
	}
}

func (p *process) fail(err error) { p.mu.Lock(); p.failure = err; p.mu.Unlock() }
func (p *process) err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failure != nil {
		return p.failure
	}
	return llm.ProviderUnavailable
}

func (c *Client) stop(p *process) {
	_ = p.in.Close()
	_ = p.cmd.Process.Kill()
	<-p.wait
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		if c.p != nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			_ = c.callProcess(ctx, c.p, "bridge.shutdown", struct{}{}, nil)
			cancel()
			c.stop(c.p)
		}
	}
	return nil
}

func (c *Client) callProcess(ctx context.Context, p *process, method string, params, out any) error {
	if ctx.Err() != nil {
		return llm.ProviderUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	id := c.next.Add(1)
	data, err := json.Marshal(params)
	if err != nil {
		return llm.InvalidProviderResponse
	}
	ch := make(chan frame, 1)
	p.mu.Lock()
	p.pending[id] = ch
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.pending, id); p.mu.Unlock() }()
	p.write.Lock()
	err = json.NewEncoder(p.in).Encode(frame{JSONRPC: "2.0", ID: id, Method: method, Params: data})
	p.write.Unlock()
	if err != nil {
		return llm.ProviderUnavailable
	}
	var f frame
	select {
	case f = <-ch:
	case <-ctx.Done():
		return llm.ProviderUnavailable
	case <-p.done:
		select {
		case f = <-ch:
		default:
			return p.err()
		}
	}
	if f.Error != nil {
		return llm.NormalizeError(f.Error.Message)
	}
	if out != nil && json.Unmarshal(f.Result, out) != nil {
		return llm.InvalidProviderResponse
	}
	return nil
}

func (c *Client) call(ctx context.Context, method string, params, out any) error {
	p, err := c.ensure(ctx)
	if err != nil {
		return err
	}
	return c.callProcess(ctx, p, method, params, out)
}

func (c *Client) Generate(ctx context.Context, r llm.GenerationRequest, sink func(llm.GenerationEvent)) (llm.GenerationResult, error) {
	return c.generate(ctx, "generation.start", r, sink)
}
func (c *Client) Continue(ctx context.Context, r llm.GenerationRequest, sink func(llm.GenerationEvent)) (llm.GenerationResult, error) {
	return c.generate(ctx, "generation.continue", r, sink)
}

func (c *Client) generate(ctx context.Context, method string, r llm.GenerationRequest, sink func(llm.GenerationEvent)) (llm.GenerationResult, error) {
	if ctx.Err() != nil {
		return llm.GenerationResult{}, llm.GenerationCancelled
	}
	if r.GenerationID == "" || r.RunID == "" || r.SessionID != r.RunID || r.Snapshot.SnapshotID == "" || r.Snapshot.MaxOutputTokens <= 0 {
		return llm.GenerationResult{}, llm.InvalidProviderResponse
	}
	p, err := c.ensure(ctx)
	if err != nil {
		return llm.GenerationResult{}, err
	}
	ch := make(chan llm.GenerationEvent, 64)
	p.mu.Lock()
	if p.events[r.GenerationID] != nil {
		p.mu.Unlock()
		return llm.GenerationResult{}, llm.InvalidProviderResponse
	}
	p.events[r.GenerationID] = ch
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.events, r.GenerationID); p.mu.Unlock() }()
	if err := c.callProcess(ctx, p, method, r, nil); err != nil {
		return llm.GenerationResult{}, err
	}
	result := llm.GenerationResult{ToolCalls: []llm.ToolCall{}}
	seen := map[int64][32]byte{}
	var last int64
	limit := MaxOutputBytes
	if r.Snapshot.MaxOutputTokens < int64(limit/4) {
		limit = int(r.Snapshot.MaxOutputTokens * 4)
	}
	for {
		if ctx.Err() != nil {
			cancelCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
			err := c.callProcess(cancelCtx, p, "generation.cancel", map[string]string{"generation_id": r.GenerationID}, nil)
			stop()
			if err != nil {
				return llm.GenerationResult{}, err
			}
			return llm.GenerationResult{}, llm.GenerationCancelled
		}
		var e llm.GenerationEvent
		select {
		case <-ctx.Done():
			continue
		case <-p.done:
			select {
			case e = <-ch:
			default:
				return llm.GenerationResult{}, p.err()
			}
		case e = <-ch:
		}
		encoded, _ := json.Marshal(e)
		digest := sha256.Sum256(encoded)
		if prior, ok := seen[e.Sequence]; ok {
			if prior != digest {
				return llm.GenerationResult{}, llm.InvalidProviderResponse
			}
			continue
		}
		if e.Sequence != last+1 || e.Sequence > 4096 {
			return llm.GenerationResult{}, llm.InvalidProviderResponse
		}
		last = e.Sequence
		seen[last] = digest
		switch e.Kind {
		case "text_delta":
			remaining := limit - len(result.Text)
			if len(e.Text) > remaining {
				for remaining > 0 && !utf8.ValidString(e.Text[:remaining]) {
					remaining--
				}
				e.Text = e.Text[:remaining]
				result.Clipped = true
			}
			result.Text += e.Text
		case "tool_call":
			if e.ToolCall == nil || e.ToolCall.ID == "" || e.ToolCall.Name == "" || len(result.ToolCalls) >= 32 || len(e.ToolCall.Arguments) > MaxOutputBytes {
				return llm.GenerationResult{}, llm.InvalidProviderResponse
			}
			result.ToolCalls = append(result.ToolCalls, *e.ToolCall)
		case "completed":
			if sink != nil {
				sink(e)
			}
			return result, nil
		case "error":
			return llm.GenerationResult{}, llm.NormalizeError(e.ErrorCode)
		default:
			return llm.GenerationResult{}, llm.InvalidProviderResponse
		}
		if sink != nil {
			sink(e)
		}
	}
}

func (c *Client) Cancel(ctx context.Context, id string) error {
	return c.call(ctx, "generation.cancel", map[string]string{"generation_id": id}, nil)
}
func (c *Client) Dispose(ctx context.Context, id string) error {
	return c.call(ctx, "session.dispose", map[string]string{"session_id": id}, nil)
}
func (c *Client) Start(ctx context.Context) (llm.AuthState, error) {
	var s llm.AuthState
	err := c.call(ctx, "auth.start", struct{}{}, &s)
	return s, err
}
func (c *Client) ContinueAuth(ctx context.Context, input string) (llm.AuthState, error) {
	var s llm.AuthState
	err := c.call(ctx, "auth.continue", map[string]string{"input": input}, &s)
	return s, err
}
func (c *Client) Refresh(ctx context.Context) (llm.AuthState, error) {
	var s llm.AuthState
	err := c.call(ctx, "auth.refresh", struct{}{}, &s)
	return s, err
}
func (c *Client) Logout(ctx context.Context) error {
	return c.call(ctx, "auth.logout", struct{}{}, nil)
}
func (c *Client) Status(ctx context.Context) (llm.AuthState, error) {
	var s llm.AuthState
	err := c.call(ctx, "auth.status", struct{}{}, &s)
	return s, err
}
func (c *Client) List(ctx context.Context) ([]llm.CatalogModel, error) {
	var s []llm.CatalogModel
	err := c.call(ctx, "models.list", struct{}{}, &s)
	return s, err
}
func (c *Client) RefreshCatalog(ctx context.Context) ([]llm.CatalogModel, error) {
	var s []llm.CatalogModel
	err := c.call(ctx, "models.refresh", struct{}{}, &s)
	return s, err
}

var _ llm.LLMProvider = (*Client)(nil)
var _ llm.ProviderAuth = (*Client)(nil)
var _ llm.ModelCatalog = (*Client)(nil)

// Run calls the versioned probe RPC for provider-side connection checks, then
// independently exercises the generation contract. Catalog claims cannot pass.
func (c *Client) Run(ctx context.Context, p llm.Profile) (llm.ProbeResult, error) {
	if err := c.call(ctx, "model.probe", p, nil); err != nil {
		return llm.ProbeResult{}, err
	}
	return (llm.Prober{Provider: c}).Run(ctx, p)
}

var _ llm.ModelProbe = (*Client)(nil)
