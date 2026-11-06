package webaccess

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/z-chenhao/assistant/internal/llm"
)

const ContentName = "builtin__fetch_content"
const ReadName = "builtin__get_search_content"
const CheckName = "builtin__source_check"

type ModelRequest struct {
	Kind      string          `json:"kind"`
	Body      json.RawMessage `json:"body,omitempty"`
	Question  string          `json:"question,omitempty"`
	PageText  string          `json:"pageText,omitempty"`
	SourceURL string          `json:"sourceUrl,omitempty"`
	Model     string          `json:"model,omitempty"`
	Messages  []llm.Message   `json:"messages,omitempty"`
	Image     *Image          `json:"image,omitempty"`
}
type Image struct {
	Data     string `json:"data"`
	MIMEType string `json:"mimeType"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}
type HostResult struct {
	llm.GenerationResult
	Image *Image `json:"image,omitempty"`
}
type Input struct {
	Name string
	Data []byte
}
type PiResult struct {
	Content []struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		Data     string `json:"data,omitempty"`
		MIMEType string `json:"mimeType,omitempty"`
	} `json:"content"`
	Details    json.RawMessage `json:"details"`
	Structured json.RawMessage `json:"structuredContent,omitempty"`
	IsError    bool            `json:"isError,omitempty"`
}
type Attachment struct {
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
	Data      string `json:"data"`
}
type PiOutput struct {
	Attachments []Attachment      `json:"attachments,omitempty"`
	Result      PiResult          `json:"result"`
	Stored      []json.RawMessage `json:"stored"`
}
type RuntimeRequest struct {
	Type       string            `json:"type"`
	Name       string            `json:"name"`
	Args       json.RawMessage   `json:"args"`
	Options    json.RawMessage   `json:"options"`
	Restore    []json.RawMessage `json:"restore,omitempty"`
	Model      *RuntimeModel     `json:"model,omitempty"`
	RunID      string            `json:"runId"`
	Input      *Input            `json:"-"`
	LocalPaths []string          `json:"localPaths,omitempty"`
}
type RuntimeModel struct {
	ID            string `json:"id"`
	Provider      string `json:"provider"`
	API           string `json:"api"`
	BaseURL       string `json:"baseUrl"`
	OAuth         bool   `json:"oauth"`
	ContextWindow int64  `json:"contextWindow"`
	MaxTokens     int64  `json:"maxTokens"`
}

var envName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,127}$`)

func (c Config) Options() (json.RawMessage, error) {
	raw := c.OptionsJSON
	if raw == "" {
		raw = "{}"
	}
	var object map[string]json.RawMessage
	if len(raw) > 65536 || json.Unmarshal([]byte(raw), &object) != nil || object == nil {
		return nil, errors.New("invalid Web Access options_json")
	}
	var value any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return nil, errors.New("invalid Web Access options_json")
	}
	var check func(any) bool
	check = func(v any) bool {
		switch x := v.(type) {
		case map[string]any:
			for key, item := range x {
				lower := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
				if s, ok := item.(string); ok && (strings.Contains(lower, "apikey") || strings.Contains(lower, "password") || lower == "authorization" || strings.Contains(lower, "cookie") || strings.HasSuffix(lower, "token") || strings.HasSuffix(lower, "secret")) {
					if !strings.HasPrefix(s, "$") || !envName.MatchString(strings.TrimPrefix(s, "$")) {
						return false
					}
				}
				if !check(item) {
					return false
				}
			}
		case string:
			if strings.HasPrefix(x, "http://") || strings.HasPrefix(x, "https://") || strings.HasPrefix(x, "socks") {
				u, e := url.Parse(x)
				if e != nil || u.User != nil {
					return false
				}
			}
		case []any:
			for _, item := range x {
				if !check(item) {
					return false
				}
			}
		}
		return true
	}
	if !check(value) {
		return nil, errors.New("web access credentials require $ENV references and credential_refs")
	}
	for name, ref := range c.CredentialRefs {
		if !envName.MatchString(name) || ref == "" || !(strings.HasSuffix(name, "_KEY") || strings.HasSuffix(name, "_TOKEN") || strings.HasSuffix(name, "_SECRET") || strings.HasSuffix(name, "_PASSWORD")) || strings.HasPrefix(name, "NODE_") || strings.HasPrefix(name, "PI_") {
			return nil, errors.New("invalid Web Access credential_refs")
		}
	}
	return json.RawMessage(raw), nil
}

// One invocation owns one isolated process and scratch root. Result state is
// returned to the Host for scoped Artifact persistence, never a shared Pi cache.
func (c Config) Invoke(ctx context.Context, request RuntimeRequest, secrets llm.SecretStore, model func(context.Context, string, ModelRequest) (llm.GenerationResult, error)) (PiOutput, error) {
	var out PiOutput
	if !filepath.IsAbs(c.RuntimeCommand) || !filepath.IsAbs(c.RuntimeScript) {
		return out, errors.New("WEB_RUNTIME_UNAVAILABLE")
	}
	options, err := c.Options()
	if err != nil {
		return out, err
	}
	root, err := os.MkdirTemp("", "steward-web-access-")
	if err != nil {
		return out, err
	}
	defer os.RemoveAll(root)
	request.LocalPaths = nil
	if request.Input != nil {
		if len(request.Input.Data) > 50<<20 {
			return out, errors.New("RESOURCE_LIMIT")
		}
		path := filepath.Join(root, "input"+filepath.Ext(filepath.Base(request.Input.Name)))
		if err = os.WriteFile(path, request.Input.Data, 0600); err != nil {
			return out, err
		}
		var args map[string]json.RawMessage
		if json.Unmarshal(request.Args, &args) != nil {
			return out, errors.New("INVALID_ARGUMENT")
		}
		delete(args, "artifactRef")
		args["url"], _ = json.Marshal(path)
		request.Args, _ = json.Marshal(args)
		request.LocalPaths = []string{path}
	}
	command := exec.CommandContext(ctx, c.RuntimeCommand, c.RuntimeScript)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.WaitDelay = time.Second
	command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }
	command.Dir = root
	bin := filepath.Join(root, "bin")
	if err = os.Mkdir(bin, 0700); err != nil {
		return out, err
	}
	for _, name := range []string{"git", "gh", "ffmpeg", "yt-dlp"} {
		if path, e := exec.LookPath(name); e == nil {
			_ = os.Symlink(path, filepath.Join(bin, name))
		}
	}
	command.Env = []string{"HOME=" + root, "XDG_CONFIG_HOME=" + root, "PI_CODING_AGENT_DIR=" + root, "PI_WEB_ACCESS_CACHE_ROOT=" + root, "TMPDIR=" + root, "LANG=C.UTF-8", "PATH=" + bin + ":" + filepath.Dir(c.RuntimeCommand) + ":/usr/bin:/bin"}
	// Resolve only named references. The entire parent environment (including
	// provider tokens, Pi/Codex caches and credential helper settings) is absent.
	for name, ref := range c.CredentialRefs {
		if secrets == nil {
			return out, llm.AuthRequired
		}
		value, e := secrets.Get(ctx, ref)
		if e != nil {
			return out, llm.AuthRequired
		}
		if len(value) > 16384 || strings.ContainsRune(string(value), 0) {
			return out, llm.AuthRequired
		}
		command.Env = append(command.Env, name+"="+string(value))
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		return out, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return out, err
	}
	command.Stderr = io.Discard // Upstream diagnostics can contain private URLs.
	if err = command.Start(); err != nil {
		return out, errors.New("WEB_RUNTIME_UNAVAILABLE")
	}
	defer func() { stdin.Close(); _ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL); _ = command.Wait() }()
	request.Type = "invoke"
	request.Options = options
	encoder := json.NewEncoder(stdin)
	if err = encoder.Encode(request); err != nil {
		return out, err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 65536), 32<<20)
	work, stop := context.WithCancel(ctx)
	var group sync.WaitGroup
	var mu sync.Mutex
	pending := map[string]context.CancelFunc{}
	seen := map[string]bool{}
	var fatal error
	defer func() { stop(); stdin.Close(); _ = command.Cancel(); group.Wait() }()
	callbacks := 0
	for scanner.Scan() {
		var frame struct {
			Type        string            `json:"type"`
			ID          string            `json:"id"`
			Request     ModelRequest      `json:"request"`
			Result      PiResult          `json:"result"`
			Stored      []json.RawMessage `json:"stored"`
			Attachments []Attachment      `json:"attachments"`
			Code        string            `json:"code"`
		}
		if json.Unmarshal(scanner.Bytes(), &frame) != nil {
			return out, errors.New("WEB_RUNTIME_PROTOCOL")
		}
		switch frame.Type {
		case "model_request":
			callbacks++
			if callbacks > 64 || (model == nil && frame.Request.Kind != "resize_image") {
				return out, errors.New("WEB_MODEL_UNAVAILABLE")
			}
			callCtx, callStop := context.WithCancel(work)
			mu.Lock()
			duplicate := seen[frame.ID]
			seen[frame.ID] = true
			pending[frame.ID] = callStop
			mu.Unlock()
			if duplicate || frame.ID == "" {
				callStop()
				return out, errors.New("WEB_RUNTIME_PROTOCOL")
			}
			group.Add(1)
			go func(id string, request ModelRequest) {
				defer group.Done()
				defer callStop()
				var result HostResult
				var e error
				if request.Kind == "resize_image" {
					result.Image, e = resizeImage(request.Image)
				} else {
					result.GenerationResult, e = model(callCtx, id, request)
				}
				reply := struct {
					Type   string     `json:"type"`
					ID     string     `json:"id"`
					Result HostResult `json:"result"`
					Error  string     `json:"error,omitempty"`
				}{Type: "model_result", ID: id, Result: result}
				if e != nil {
					reply.Error = e.Error()
				}
				mu.Lock()
				defer mu.Unlock()
				delete(pending, id)
				if e != nil && (errors.Is(e, llm.AuthRequired) || strings.HasPrefix(e.Error(), "BUDGET_") || ctx.Err() != nil) {
					fatal = e
					_ = command.Cancel()
					return
				}
				if e := encoder.Encode(reply); e != nil && fatal == nil {
					fatal = errors.New("WEB_RUNTIME_PROTOCOL")
					_ = command.Cancel()
				}
			}(frame.ID, frame.Request)
		case "model_cancel":
			mu.Lock()
			cancel := pending[frame.ID]
			mu.Unlock()
			if cancel != nil {
				cancel()
			}
		case "result":
			mu.Lock()
			e := fatal
			mu.Unlock()
			if e != nil {
				return out, e
			}
			out.Attachments = frame.Attachments
			out.Result = frame.Result
			out.Stored = frame.Stored
			return out, nil
		case "error":
			return out, errors.New("WEB_ACCESS_FAILED")
		default:
			return out, errors.New("WEB_RUNTIME_PROTOCOL")
		}
	}
	if ctx.Err() != nil {
		return out, llm.GenerationCancelled
	}
	if errors.Is(scanner.Err(), bufio.ErrTooLong) {
		return out, errors.New("RESOURCE_LIMIT")
	}
	mu.Lock()
	e := fatal
	mu.Unlock()
	if e != nil {
		return out, e
	}
	return out, errors.New("WEB_RUNTIME_PROTOCOL")
}
