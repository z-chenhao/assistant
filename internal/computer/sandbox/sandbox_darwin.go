// Package sandbox owns the interim M3 process boundary. All scopes and commands
// come from the Host; models cannot select commands, environments or mounts.
package sandbox

import (
	"context"
	"os/exec"
	"strconv"
)

func Command(ctx context.Context, executable string, args []string, readRoots []string, writeRoot, workingDir string) (*exec.Cmd, error) {
	q := strconv.Quote
	profile := `(version 1)(deny default)(allow process-fork)(allow process-exec)(allow sysctl-read)(allow file-read-metadata)(allow file-read* (literal "/") (subpath "/usr") (subpath "/bin") (subpath "/System") (subpath "/Library/Developer") (subpath "/Applications/Xcode.app/Contents/Developer") (literal ` + q(executable) + `) (subpath ` + q(writeRoot) + `) (literal "/dev/null") (literal "/dev/urandom")`
	for _, root := range readRoots {
		profile += `(subpath ` + q(root) + `)`
	}
	profile += `)(allow file-write* (subpath ` + q(writeRoot) + `) (literal "/dev/null"))`
	return exec.CommandContext(ctx, "/usr/bin/sandbox-exec", append([]string{"-p", profile, executable}, args...)...), nil
}
