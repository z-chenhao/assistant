package workspace

import (
	"context"
	processsandbox "github.com/z-chenhao/erii/internal/computer/sandbox"
	"os/exec"
	"path/filepath"
)

func platformSandbox(ctx context.Context, dir, repo, goroot string) (*exec.Cmd, error) {
	// All mutable state is a disposable copy, never the source checkout.
	// The platform adapter denies network and host user data by default.
	cmd, err := processsandbox.Command(ctx, filepath.Join(goroot, "bin/go"), []string{"test", "-race", "./..."}, []string{goroot}, dir, repo)
	if err != nil {
		return nil, err
	}
	cmd.Dir = repo
	cmd.Env = []string{"PATH=" + filepath.Join(goroot, "bin") + ":/usr/bin:/bin", "HOME=" + dir, "TMPDIR=" + dir, "GOCACHE=" + filepath.Join(dir, "cache"), "GOPATH=" + filepath.Join(dir, "gopath"), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local", "GOENV=off", "CGO_ENABLED=1"}
	return cmd, nil
}
