package workspace

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/z-chenhao/erii/internal/tool"
)

type limitedBuffer struct {
	data     []byte
	limit    int
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - len(b.data)
	if len(p) > remaining {
		p = p[:remaining]
		b.overflow = true
	}
	b.data = append(b.data, p...)
	return n, nil
}

func test(ctx context.Context, root *os.Root) tool.Result {
	dir, err := os.MkdirTemp("", "erii-coding-test-")
	if err != nil {
		return tool.Failure("PROVIDER_UNAVAILABLE")
	}
	defer os.RemoveAll(dir)
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return tool.Failure("PROVIDER_UNAVAILABLE")
	}
	repo := filepath.Join(dir, "repo")
	if os.Mkdir(repo, 0700) != nil {
		return tool.Failure("PROVIDER_UNAVAILABLE")
	}
	total, count := 0, 0
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, e error) error {
		if e != nil || ctx.Err() != nil {
			return invalid
		}
		count++
		if count > 4096 {
			return invalid
		}
		if d.IsDir() {
			if d.Name() == ".git" || strings.HasPrefix(d.Name(), ".erii-") {
				return fs.SkipDir
			}
			if p == "." {
				return nil
			}
			return os.MkdirAll(filepath.Join(repo, p), 0700)
		}
		doc, e := read(root, p)
		if e != nil {
			return invalid
		}
		total += len(doc.Text)
		if total > 8<<20 {
			return invalid
		}
		return os.WriteFile(filepath.Join(repo, p), []byte(doc.Text), 0600)
	})
	if err != nil {
		return tool.Failure("PERMISSION_DENIED")
	}
	cmd, err := sandbox(ctx, dir, repo)
	if err != nil {
		return tool.Failure("PROVIDER_UNAVAILABLE")
	}
	out := &limitedBuffer{limit: 60000}
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.WaitDelay = time.Second
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	err = cmd.Start()
	if err == nil {
		defer syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		err = cmd.Wait()
	}
	if ctx.Err() != nil {
		return tool.Failure("CANCELLED")
	}
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return tool.Failure("PROVIDER_UNAVAILABLE")
		}
	}
	text := strings.ReplaceAll(string(out.data), dir, "<sandbox>")
	return result(struct {
		Passed    bool   `json:"passed"`
		Command   string `json:"command"`
		Output    string `json:"output"`
		Truncated bool   `json:"truncated"`
	}{err == nil, "go test -race ./...", text, out.overflow})
}

// The platform files contain the enforceable execution boundary. There is no
// unsandboxed fallback and no shell, command, environment or mount model input.
func sandbox(ctx context.Context, dir, repo string) (*exec.Cmd, error) {
	return platformSandbox(ctx, dir, repo, runtime.GOROOT())
}
