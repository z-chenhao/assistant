// Package bridge owns the versioned, private stdio Provider RPC. It never
// exposes child stderr or provider response bodies as errors.
package bridge

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
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
	cmd      *exec.Cmd
	in       *os.File
	write    chan struct{}
	stopOnce sync.Once
	mu       sync.Mutex
	pending  map[uint64]chan frame
	events   map[string]chan llm.GenerationEvent
	done     chan struct{}
	wait     chan struct{}
	failure  error
}

type Client struct {
	mu           sync.Mutex
	path         string
	args         []string
	p            *process
	closed       bool
	initializing *initialization
	closeDone    chan struct{}
	next         atomic.Uint64
}

type initialization struct {
	done chan struct{}
	err  error // published by closing done
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
	for {
		if ctx.Err() != nil {
			return nil, llm.ProviderUnavailable
		}
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return nil, llm.ProviderUnavailable
		}
		if pending := c.initializing; pending != nil {
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, llm.ProviderUnavailable
			case <-pending.done:
			}
			if ctx.Err() != nil {
				return nil, llm.ProviderUnavailable
			}
			if pending.err != nil {
				return nil, pending.err
			}
			continue
		}
		if c.p != nil {
			select {
			case <-c.p.done:
			default:
				p := c.p
				c.mu.Unlock()
				return p, nil
			}
		}
		previous := c.p
		pending := &initialization{done: make(chan struct{})}
		c.p = nil
		c.initializing = pending
		c.mu.Unlock()
		// No process I/O or reap runs under the lifecycle state lock. Waiters can
		// abandon this initialization independently, and Close can stop its child.
		if previous != nil {
			c.stop(previous)
		}
		p, err := c.startProcess(ctx)
		c.mu.Lock()
		if c.closed {
			err = llm.ProviderUnavailable
		}
		if err != nil {
			c.p = nil
		}
		pending.err = err
		c.initializing = nil
		close(pending.done)
		c.mu.Unlock()
		if err != nil {
			return nil, err
		}
		return p, nil
	}
}

func (c *Client) startProcess(ctx context.Context) (*process, error) {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed || ctx.Err() != nil {
		return nil, llm.ProviderUnavailable
	}
	cmd := exec.Command(c.path, c.args...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	cmd.Dir = os.TempDir()
	cmd.Stderr = io.Discard
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, llm.ProviderUnavailable
	}
	in, ok := input.(*os.File)
	if !ok {
		_ = input.Close()
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
	p := &process{cmd: cmd, in: in, write: make(chan struct{}, 1), pending: map[uint64]chan frame{}, events: map[string]chan llm.GenerationEvent{}, done: make(chan struct{}), wait: make(chan struct{})}
	go p.read(out)
	// Wait only after read has consumed stdout. Close always reaps this child.
	go func() { <-p.done; _ = cmd.Wait(); close(p.wait) }()
	// Publish before handshake I/O so forced shutdown can reach this process.
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		c.stop(p)
		return nil, llm.ProviderUnavailable
	}
	c.p = p
	c.mu.Unlock()
	var hello struct {
		ProtocolVersion int `json:"protocol_version"`
	}
	if err := c.callProcess(ctx, p, "bridge.initialize", map[string]int{"protocol_version": ProtocolVersion}, &hello); err != nil || hello.ProtocolVersion != ProtocolVersion {
		c.stop(p)
		return nil, llm.ModelIncompatible
	}
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
	p.stopOnce.Do(func() { _ = p.in.Close(); _ = p.cmd.Process.Kill() })
	<-p.wait
}

func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		done := c.closeDone
		c.mu.Unlock()
		<-done
		return nil
	}
	c.closed = true
	c.closeDone = make(chan struct{})
	done, p, pending := c.closeDone, c.p, c.initializing
	c.mu.Unlock()
	if p != nil {
		// An initializer cannot process shutdown before its handshake completes.
		// Force it immediately; established peers receive bounded graceful close.
		if pending == nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			_ = c.callProcess(ctx, p, "bridge.shutdown", struct{}{}, nil)
			cancel()
		}
		c.stop(p)
	}
	if pending != nil {
		<-pending.done
	}
	close(done)
	return nil
}

var errAcknowledgementTimeout = errors.New("provider RPC acknowledgement timeout")

func (c *Client) callProcess(ctx context.Context, p *process, method string, params, out any) error {
	_, err := c.exchange(ctx, p, method, params, out)
	if err == errAcknowledgementTimeout {
		return llm.ProviderUnavailable
	}
	return err
}

// exchange reports complete dispatch separately from acknowledgement. A timed
// out response does not mean a generation request was never accepted by the peer.
func (c *Client) exchange(ctx context.Context, p *process, method string, params, out any) (bool, error) {
	if ctx.Err() != nil {
		return false, llm.ProviderUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	id := c.next.Add(1)
	data, err := json.Marshal(params)
	if err != nil {
		return false, llm.InvalidProviderResponse
	}
	wire, err := json.Marshal(frame{JSONRPC: "2.0", ID: id, Method: method, Params: data})
	if err != nil || len(wire) >= MaxFrameBytes {
		return false, llm.InvalidProviderResponse
	}
	wire = append(wire, '\n')
	ch := make(chan frame, 1)
	p.mu.Lock()
	p.pending[id] = ch
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.pending, id); p.mu.Unlock() }()
	select {
	case p.write <- struct{}{}:
	case <-ctx.Done():
		return false, llm.ProviderUnavailable
	case <-p.done:
		return false, p.err()
	}
	// One writer owns pipe deadlines. Cancellation interrupts the actual Write,
	// including a peer that has stopped reading. Join the callback before the next
	// writer can reset the deadline, so it cannot interrupt a subsequent request.
	err = c.write(ctx, p, wire)
	<-p.write
	if err != nil {
		return false, err
	}
	var f frame
	select {
	case f = <-ch:
	case <-ctx.Done():
		return true, errAcknowledgementTimeout
	case <-p.done:
		select {
		case f = <-ch:
		default:
			return true, p.err()
		}
	}
	if f.Error != nil {
		return true, llm.NormalizeError(f.Error.Message)
	}
	if out != nil && json.Unmarshal(f.Result, out) != nil {
		return true, llm.InvalidProviderResponse
	}
	return true, nil
}

func (c *Client) write(ctx context.Context, p *process, wire []byte) error {
	if ctx.Err() != nil {
		return llm.ProviderUnavailable
	}
	deadline, _ := ctx.Deadline()
	if err := p.in.SetWriteDeadline(deadline); err != nil {
		c.stop(p)
		return llm.ProviderUnavailable
	}
	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = p.in.SetWriteDeadline(time.Now()); close(interrupted) })
	_, err := p.in.Write(wire)
	if !stop() {
		<-interrupted
	}
	_ = p.in.SetWriteDeadline(time.Time{})
	if err != nil {
		// A partial JSON frame makes the stream unusable. Terminate this process;
		// do not replay this request or let another writer append to its prefix.
		c.stop(p)
		return llm.ProviderUnavailable
	}
	return nil
}

func (c *Client) cleanupGeneration(ctx context.Context, p *process, id string) error {
	cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer stop()
	err := c.callProcess(cleanup, p, "generation.cancel", map[string]string{"generation_id": id}, nil)
	if err != nil {
		c.stop(p)
	}
	return err
}

func (c *Client) cancelGeneration(ctx context.Context, p *process, id string) error {
	if err := c.cleanupGeneration(ctx, p, id); err != nil {
		return errors.Join(llm.GenerationCancelled, err)
	}
	return llm.GenerationCancelled
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
		if ctx.Err() != nil {
			return llm.GenerationResult{}, llm.GenerationCancelled
		}
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
	dispatched, err := c.exchange(ctx, p, method, r, nil)
	if ctx.Err() != nil {
		if dispatched {
			return llm.GenerationResult{}, c.cancelGeneration(ctx, p, r.GenerationID)
		}
		return llm.GenerationResult{}, llm.GenerationCancelled
	}
	if err != nil {
		if dispatched && err == errAcknowledgementTimeout {
			if cleanupErr := c.cleanupGeneration(ctx, p, r.GenerationID); cleanupErr != nil {
				return llm.GenerationResult{}, errors.Join(llm.ProviderUnavailable, cleanupErr)
			}
			return llm.GenerationResult{}, llm.ProviderUnavailable
		}
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
			return llm.GenerationResult{}, c.cancelGeneration(ctx, p, r.GenerationID)
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
