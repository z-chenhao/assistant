package sandbox

import (
	"context"
	"os/exec"
)

func Command(ctx context.Context, executable string, args []string, readRoots []string, writeRoot, workingDir string) (*exec.Cmd, error) {
	bin, err := exec.LookPath("bwrap")
	if err != nil {
		return nil, err
	}
	flags := []string{"--unshare-all", "--die-with-parent", "--new-session", "--cap-drop", "ALL", "--ro-bind", "/usr", "/usr", "--symlink", "usr/bin", "/bin", "--symlink", "usr/lib", "/lib", "--symlink", "usr/lib64", "/lib64", "--proc", "/proc", "--dev", "/dev", "--dir", "/tmp", "--bind", writeRoot, writeRoot}
	for _, root := range readRoots {
		flags = append(flags, "--ro-bind", root, root)
	}
	flags = append(flags, "--ro-bind", executable, executable, "--chdir", workingDir, "--", executable)
	flags = append(flags, args...)
	return exec.CommandContext(ctx, bin, flags...), nil
}
