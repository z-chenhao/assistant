package environment

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func localFixture(t *testing.T) Local {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	return Local{Config: Config{Root: root}}
}
func execute(t *testing.T, l Local, name string, args any) toolValue {
	t.Helper()
	raw, _ := json.Marshal(args)
	r := l.Execute(context.Background(), "builtin__"+name, string(raw))
	if r.Error != nil {
		t.Fatalf("%s: %s", name, r.Error.Code)
	}
	var value toolValue
	if err := json.Unmarshal([]byte(r.Text), &value); err != nil {
		t.Fatal(err, r.Text)
	}
	return value
}

type toolValue struct {
	Text, Hash, Output string
	ExitCode           int  `json:"exit_code"`
	TimedOut           bool `json:"timed_out"`
	Truncated          bool
}

func TestPersistentReadBashEditWriteAndHostIsolation(t *testing.T) {
	l := localFixture(t)
	t.Setenv("COLLIE_SECRET_SENTINEL", "host-credential-not-exported")
	host := filepath.Join(t.TempDir(), "host-only")
	if err := os.WriteFile(host, []byte("HOST_PRIVATE"), 0600); err != nil {
		t.Fatal(err)
	}
	execute(t, l, "write", map[string]any{"path": "nested/file.txt", "content": "user line\nagent one\n"})
	read := execute(t, l, "read", map[string]any{"path": "nested/file.txt", "offset": 2, "limit": 1})
	if read.Text != "agent one\n" || len(read.Hash) != 64 {
		t.Fatal(read)
	}
	execute(t, l, "edit", map[string]any{"path": "nested/file.txt", "old_text": "one", "new_text": "two", "before_hash": read.Hash})
	read = execute(t, l, "read", map[string]any{"path": "nested/file.txt"})
	execute(t, l, "edit", map[string]any{"path": "nested/file.txt", "old_text": "two", "new_text": "three", "before_hash": read.Hash})
	result := execute(t, l, "bash", map[string]any{"command": "cat nested/file.txt; test ! -e " + host + "; test -z \"$COLLIE_SECRET_SENTINEL\"; printf 'shell file' > shell.txt"})
	if result.ExitCode != 0 || !strings.Contains(result.Output, "user line\nagent three") {
		t.Fatal(result)
	}
	if got := execute(t, l, "read", map[string]any{"path": "shell.txt"}); got.Text != "shell file" {
		t.Fatal(got)
	}
	for name, args := range map[string]string{
		"read":  `{"path":"../host-only"}`,
		"edit":  `{"path":"nested/file.txt","old_text":"three","new_text":"bad","before_hash":"stale"}`,
		"write": `{"path":"shell.txt","content":"blind overwrite"}`,
	} {
		if r := l.Execute(context.Background(), "builtin__"+name, args); r.Error == nil || r.Error.Code != "PERMISSION_DENIED" {
			t.Fatal(name, r)
		}
	}
	if err := os.Symlink(host, filepath.Join(l.Config.Root, "link")); err != nil {
		t.Fatal(err)
	}
	if r := l.Execute(context.Background(), "builtin__read", `{"path":"link"}`); r.Error == nil {
		t.Fatal("symlink followed")
	}
	if err := os.Link(filepath.Join(l.Config.Root, "shell.txt"), filepath.Join(l.Config.Root, "hard")); err != nil {
		t.Fatal(err)
	}
	if r := l.Execute(context.Background(), "builtin__read", `{"path":"hard"}`); r.Error == nil {
		t.Fatal("hard link followed")
	}
}

func TestBashDeadlineOutputBoundsAndArtifactBytes(t *testing.T) {
	l := localFixture(t)
	result := execute(t, l, "bash", map[string]any{"command": "sleep 5; echo late > late.txt", "timeout": 1})
	if !result.TimedOut || result.ExitCode == 0 {
		t.Fatal(result)
	}
	if _, err := os.Stat(filepath.Join(l.Config.Root, "late.txt")); !os.IsNotExist(err) {
		t.Fatal("timed out child survived")
	}
	result = execute(t, l, "bash", map[string]any{"command": "yes x | head -c 100000"})
	if !result.Truncated || len(result.Output) > 48000 {
		t.Fatal("unbounded output", len(result.Output))
	}
	// Redirecting stdout must not be mistaken for process completion.
	result = execute(t, l, "bash", map[string]any{"command": "printf before > redirected.txt; exec >/dev/null 2>&1; sleep .2; printf after >> redirected.txt"})
	if result.ExitCode != 0 {
		t.Fatal(result)
	}
	if got := execute(t, l, "read", map[string]any{"path": "redirected.txt"}); got.Text != "beforeafter" {
		t.Fatal("redirected process killed before completion", got)
	}
	data := []byte{0, 255, 1, 2}
	if err := os.WriteFile(filepath.Join(l.Config.Root, "binary.bin"), data, 0600); err != nil {
		t.Fatal(err)
	}
	r := l.Execute(context.Background(), "builtin__read", `{"path":"binary.bin"}`)
	if r.Error != nil || r.Output == nil {
		t.Fatal(r)
	}
	actual, err := io.ReadAll(r.Output.Reader)
	if err != nil || string(actual) != string(data) {
		t.Fatal(actual, err)
	}
	if strings.Contains(r.Output.ResultJSON("artifact:opaque"), "255") {
		t.Fatal("binary leaked into model output")
	}
}

func TestNoHostExecutionFallbackWithoutSandbox(t *testing.T) {
	l := localFixture(t)
	// Linux resolves bwrap through PATH; macOS invokes its fixed system adapter.
	if _, err := os.Stat("/usr/bin/sandbox-exec"); err == nil {
		t.Skip("Linux missing-sandbox path")
	}
	t.Setenv("PATH", t.TempDir())
	r := l.Execute(context.Background(), "builtin__bash", `{"command":"touch fallback"}`)
	if r.Error == nil || r.Error.Code != "PROVIDER_UNAVAILABLE" {
		t.Fatal(r)
	}
	if _, err := os.Stat(filepath.Join(l.Config.Root, "fallback")); !os.IsNotExist(err) {
		t.Fatal("Host fallback executed")
	}
}
