package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/collie/internal/tool"
	mcpfake "github.com/z-chenhao/collie/internal/tool/mcp/fake"
)

type rewrite struct {
	io.Writer
	mode string
}

func (w rewrite) Write(b []byte) (int, error) {
	text := string(b)
	if w.mode == "underscores" {
		text = strings.ReplaceAll(text, `"name":"read"`, `"name":"click_button"`)
	}
	if w.mode == "malformed" {
		text = strings.ReplaceAll(text, `"maxLength":4096`, `"unsupported":4096`)
	}
	if w.mode == "scalar" {
		text = strings.ReplaceAll(text, `"type":"object"`, `"type":"string"`)
	}
	if w.mode == "drift" {
		text = strings.ReplaceAll(text, "Read structured fixture data", "Changed tool")
	}
	_, err := io.WriteString(w.Writer, text)
	return len(b), err
}
func TestMCPPeer(t *testing.T) {
	index := -1
	for i, a := range os.Args {
		if a == "mcp-fixture" {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	if os.Getenv("COLLIE_TEST_SECRET") != "" {
		os.Exit(3)
	}
	root := os.Args[index+1]
	var boundary struct{ Secret, Address string }
	if b, e := os.ReadFile(filepath.Join(root, "boundary")); e == nil {
		if json.Unmarshal(b, &boundary) != nil {
			os.Exit(4)
		}
		if _, e := os.ReadFile(boundary.Secret); e == nil {
			os.Exit(4)
		}
		if e := os.WriteFile(boundary.Secret, []byte("changed"), 0600); e == nil {
			os.Exit(4)
		}
		if conn, e := net.DialTimeout("tcp", boundary.Address, 200*time.Millisecond); e == nil {
			conn.Close()
			os.Exit(4)
		}
	}
	mode, _ := os.ReadFile(filepath.Join(root, "mode"))
	if mcpfake.Serve(os.Stdin, rewrite{os.Stdout, string(mode)}, root) != nil {
		os.Exit(2)
	}
	os.Exit(0)
}
func config(t *testing.T) Config {
	t.Helper()
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	return Config{Name: "fixture", Command: bin, Directory: root, Args: []string{"-test.run=^TestMCPPeer$", "--", "mcp-fixture", root}, Tools: map[string]string{"read": "read_only", "send": "external_effect"}}
}

func TestMCPWireNameKeepsServerAndToolUnderscores(t *testing.T) {
	c := config(t)
	c.Name = "browser_use"
	c.Tools = map[string]string{"click_button": "read_only"}
	if err := os.WriteFile(filepath.Join(c.Args[len(c.Args)-1], "mode"), []byte("underscores"), 0600); err != nil {
		t.Fatal(err)
	}
	defs, err := Definitions(context.Background(), c)
	if err != nil || len(defs) != 1 || defs[0].Spec.Name != "mcp__browser_use__click_button" {
		t.Fatal("MCP name was split or flattened", defs, err)
	}
	for _, invalid := range []Config{
		{Name: "browser__use", Command: c.Command, Directory: c.Directory, Tools: c.Tools},
		{Name: c.Name, Command: c.Command, Directory: c.Directory, Tools: map[string]string{"click__button": "read_only"}},
	} {
		if invalid.Validate() == nil {
			t.Fatal("ambiguous double-underscore identifier accepted", invalid.Name)
		}
	}
}
func TestDiscoveryFreezesHostPolicyAndNormalizesErrors(t *testing.T) {
	t.Setenv("COLLIE_TEST_SECRET", "secret")
	c := config(t)
	defs, err := Definitions(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := tool.NewRegistry(defs)
	if err != nil {
		t.Fatal(err)
	}
	read, _ := registry.Resolve("mcp__fixture__read")
	send, _ := registry.Resolve("mcp__fixture__send")
	if send.Spec.Permission != "external_effect" || send.Spec.ReplayPolicy != "never" || send.Spec.Risk != "high" || read.Spec.ResourceHash == "" {
		t.Fatal("server annotation overrode Host policy")
	}
	if got := read.Execute(context.Background(), `{"text":"actual response"}`); got.Text != "actual response" {
		t.Fatal(got)
	}
	if got := read.Execute(context.Background(), `{"text":"raw secret /host/path","behavior":"error"}`); got.Error == nil || got.Error.Code != "INVALID_ARGUMENT" || strings.Contains(got.ModelText(), "secret") {
		t.Fatal(got)
	}
	if tool.Validate(read.Spec.InputSchema, `{"text":"x","call_key":"forged"}`) {
		t.Fatal("forged field valid")
	}
	if keys := send.ConflictKeys(`{}`); len(keys) != 1 || keys[0] != read.ConflictKeys(`{}`)[0] {
		t.Fatal(keys)
	}
	root := c.Args[len(c.Args)-1]
	if err := os.WriteFile(filepath.Join(root, "mode"), []byte("drift"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := read.Execute(context.Background(), `{"text":"x"}`); got.Error == nil || got.Error.Code != "NON_DETERMINISTIC_REPLAY" {
		t.Fatal(got)
	}
}
func TestMalformedDiscoveryAndMissingAllowlistFailClosed(t *testing.T) {
	for _, mode := range []string{"malformed", "scalar", "missing", "permission", "name"} {
		t.Run(mode, func(t *testing.T) {
			c := config(t)
			switch mode {
			case "malformed", "scalar":
				if err := os.WriteFile(filepath.Join(c.Args[len(c.Args)-1], "mode"), []byte(mode), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing":
				c.Tools["absent"] = "read_only"
			case "permission":
				c.Tools["send"] = "safe"
			case "name":
				c.Name = "bad/name"
			}
			if _, err := Definitions(context.Background(), c); err == nil {
				t.Fatal("accepted malformed catalog")
			}
		})
	}
}
func TestCancellationAndUnknownEffect(t *testing.T) {
	c := config(t)
	defs, err := Definitions(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := tool.NewRegistry(defs)
	read, _ := r.Resolve("mcp__fixture__read")
	send, _ := r.Resolve("mcp__fixture__send")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	got := read.Execute(ctx, `{"text":"x","behavior":"hold"}`)
	if got.Error == nil || time.Since(start) > 3*time.Second {
		t.Fatal(got)
	}
	outcome := send.Invoke(context.Background(), tool.Request{Arguments: `{"text":"order","behavior":"unknown"}`, IdempotencyKey: "host-only"})
	if !outcome.Ambiguous {
		t.Fatal(outcome)
	}
	b, err := os.ReadFile(filepath.Join(c.Args[len(c.Args)-1], "mcp-effects.jsonl"))
	if err != nil || strings.Count(string(b), "order") != 1 {
		t.Fatal(string(b), err)
	}
}

func TestCallResultShapeOverStdio(t *testing.T) {
	for _, tc := range []struct {
		name, reply, text string
		valid             bool
	}{
		{"null", `null`, "", false},
		{"scalar", `"secret"`, "", false},
		{"array", `[]`, "", false},
		{"missing_content", `{}`, "", false},
		{"null_content", `{"content":null}`, "", false},
		{"object_content", `{"content":{}}`, "", false},
		{"null_part", `{"content":[null]}`, "", false},
		{"missing_type", `{"content":[{"text":"secret"}]}`, "", false},
		{"missing_text", `{"content":[{"type":"text"}]}`, "", false},
		{"null_text", `{"content":[{"type":"text","text":null}]}`, "", false},
		{"number_text", `{"content":[{"type":"text","text":42}]}`, "", false},
		{"null_error", `{"content":[],"isError":null}`, "", false},
		{"string_error", `{"content":[],"isError":"false"}`, "", false},
		{"unsupported_content", `{"content":[{"type":"image","data":"secret"}]}`, "", false},
		{"error_with_missing_text", `{"content":[{"type":"text"}],"isError":true}`, "", false},
		{"empty_content", `{"content":[]}`, "", true},
		{"empty_text", `{"content":[{"type":"text","text":""}]}`, "", true},
		{"text_parts", `{"content":[{"type":"text","text":"one"},{"type":"text","text":"two"}],"isError":false}`, "onetwo", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := config(t)
			if err := os.WriteFile(filepath.Join(c.Directory, "mcp-result.json"), []byte(tc.reply), 0600); err != nil {
				t.Fatal(err)
			}
			defs, err := Definitions(context.Background(), c)
			if err != nil {
				t.Fatal(err)
			}
			r, err := tool.NewRegistry(defs)
			if err != nil {
				t.Fatal(err)
			}
			read, _ := r.Resolve("mcp__fixture__read")
			send, _ := r.Resolve("mcp__fixture__send")
			got := read.Execute(context.Background(), `{"text":"read"}`)
			outcome := send.Invoke(context.Background(), tool.Request{Arguments: `{"text":"order"}`})
			if tc.valid {
				if got.Error != nil || got.Text != tc.text || outcome.Ambiguous || outcome.Result.Error != nil || outcome.Result.Text != tc.text {
					t.Fatalf("valid reply rejected: %+v %+v", got, outcome)
				}
			} else {
				if got.Error == nil || got.Error.Code != "PROVIDER_UNAVAILABLE" || !outcome.Ambiguous || outcome.Result.Error == nil || outcome.Result.Error.Code != "PROVIDER_UNAVAILABLE" {
					t.Fatalf("malformed post-dispatch reply accepted: %+v %+v", got, outcome)
				}
				if strings.Contains(got.ModelText(), "secret") || strings.Contains(outcome.Result.ModelText(), "secret") {
					t.Fatal("raw malformed reply leaked")
				}
			}
			b, err := os.ReadFile(filepath.Join(c.Directory, "mcp-effects.jsonl"))
			if err != nil || strings.Count(string(b), "order") != 1 {
				t.Fatal("expected exactly one external effect", string(b), err)
			}
		})
	}
}

func TestMCPProcessHasOnlyItsConfiguredDirectory(t *testing.T) {
	c := config(t)
	secret := filepath.Join(t.TempDir(), "credential")
	if err := os.WriteFile(secret, []byte("HOST_CREDENTIAL"), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	b, _ := json.Marshal(map[string]string{"Secret": secret, "Address": listener.Addr().String()})
	if err := os.WriteFile(filepath.Join(c.Directory, "boundary"), b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Definitions(context.Background(), c); err != nil {
		t.Fatal("MCP process escaped its scope or failed discovery", err)
	}
	b, err = os.ReadFile(secret)
	if err != nil || string(b) != "HOST_CREDENTIAL" {
		t.Fatal("MCP changed credential", err)
	}
}
