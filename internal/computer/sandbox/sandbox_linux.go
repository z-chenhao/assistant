package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
)

func Check() error { _, err := exec.LookPath("bwrap"); return err }

func Python3(context.Context) (string, error) { return filepath.EvalSymlinks("/usr/bin/python3") }

func PythonCommand(ctx context.Context, args []string, readRoots []string, root string) (*exec.Cmd, error) {
	interpreter, err := Python3(ctx)
	if err != nil {
		return nil, err
	}
	return Command(ctx, interpreter, args, readRoots, root, root)
}

func Command(ctx context.Context, executable string, args []string, readRoots []string, writeRoot, workingDir string) (*exec.Cmd, error) {
	bin, err := exec.LookPath("bwrap")
	if err != nil {
		return nil, err
	}
	flags := []string{"--unshare-all", "--die-with-parent", "--new-session", "--cap-drop", "ALL", "--symlink", "usr/bin", "/bin", "--symlink", "usr/lib", "/lib", "--symlink", "usr/lib64", "/lib64", "--proc", "/proc", "--dev", "/dev", "--dir", "/tmp", "--bind", writeRoot, writeRoot}
	for _, root := range []string{"/usr/bin", "/usr/lib", "/usr/lib64", "/usr/libexec", "/usr/share", "/usr/include", "/etc/alternatives"} {
		if _, e := os.Stat(root); e == nil {
			flags = append(flags, "--ro-bind", root, root)
		}
	}
	for _, root := range readRoots {
		flags = append(flags, "--ro-bind", root, root)
	}
	flags = append(flags, "--ro-bind", executable, executable, "--chdir", workingDir, "--", executable)
	flags = append(flags, args...)
	return exec.CommandContext(ctx, bin, flags...), nil
}
