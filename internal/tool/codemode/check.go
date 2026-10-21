package codemode

import (
	"bufio"
	"context"
	"encoding/json"
	"os/exec"
	"time"
)

// Check performs only the runtime handshake and graceful shutdown; it never
// evaluates a cell or admits a Host call. The probe process is always joined.
func (s *Sidecar) Check(ctx context.Context) error {
	runtimeHash, preludeHash, e := s.Fingerprint()
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.Command, s.Script)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "TZ=UTC", "LANG=C.UTF-8"}
	in, e := cmd.StdinPipe()
	if e != nil {
		return ErrProtocol
	}
	out, e := cmd.StdoutPipe()
	if e != nil {
		return ErrProtocol
	}
	if cmd.Start() != nil {
		return ErrProtocol
	}
	joined := false
	defer func() {
		in.Close()
		if !joined {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	encoder := json.NewEncoder(in)
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	if encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": "init", "method": "runtime.initialize", "params": map[string]string{"protocolVersion": ProtocolVersion, "runtimeVersion": RuntimeVersion, "preludeVersion": PreludeVersion}}) != nil || !scanner.Scan() {
		return ErrProtocol
	}
	var msg rpc
	if json.Unmarshal(scanner.Bytes(), &msg) != nil || msg.JSONRPC != "2.0" || string(msg.ID) != `"init"` || len(msg.Error) > 0 {
		return ErrProtocol
	}
	var identity struct {
		Protocol    string `json:"protocolVersion"`
		Runtime     string `json:"runtimeVersion"`
		Prelude     string `json:"preludeVersion"`
		RuntimeHash string `json:"runtimeHash"`
		PreludeHash string `json:"preludeHash"`
	}
	if json.Unmarshal(msg.Result, &identity) != nil || identity.Protocol != ProtocolVersion || identity.Runtime != RuntimeVersion || identity.Prelude != PreludeVersion || identity.RuntimeHash != runtimeHash || identity.PreludeHash != preludeHash {
		return ErrProtocol
	}
	if encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": "stop", "method": "runtime.shutdown", "params": map[string]any{}}) != nil || !scanner.Scan() {
		return ErrProtocol
	}
	if json.Unmarshal(scanner.Bytes(), &msg) != nil || string(msg.ID) != `"stop"` || len(msg.Error) > 0 {
		return ErrProtocol
	}
	in.Close()
	e = cmd.Wait()
	joined = true
	if e != nil {
		return ErrProtocol
	}
	return nil
}
