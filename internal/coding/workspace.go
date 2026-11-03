// Package coding prepares private Task working copies. Source Git metadata is
// never imported or executed; the file-tree snapshot includes owner dirty files.
package coding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/z-chenhao/assistant/internal/computer/sandbox"
	"golang.org/x/sys/unix"
)

var ErrInvalid = errors.New("code workspace unavailable or unsafe")

const maxFiles = 4096
const maxBytes = 64 << 20

type Workspace struct {
	TaskRef      string `json:"task_ref"`
	Directory    string `json:"directory,omitempty"`
	Baseline     string `json:"-"`
	BaselineHash string `json:"baseline_sha256"`
	Status       string `json:"status"`
	ErrorCode    string `json:"error_code,omitempty"`
}
type receipt struct {
	Version      int    `json:"version"`
	Scope        string `json:"scope"`
	BaselineHash string `json:"baseline_sha256"`
}
type manifestFile struct {
	Name string `json:"name"`
	Hash string `json:"hash"`
	Mode uint32 `json:"mode"`
}

// Tree validates every entry and optionally copies regular files through an
// anchored os.Root. No symlink/hardlink/device is accepted, including after work.
func Tree(ctx context.Context, source, destination string) (string, error) {
	return scanTree(ctx, source, destination, nil)
}

func scanTree(ctx context.Context, source, destination string, inventory func([]manifestFile)) (string, error) {
	root, err := os.OpenRoot(source)
	if err != nil {
		return "", ErrInvalid
	}
	defer root.Close()
	var entries []manifestFile
	visited := 0
	total := int64(0)
	var walk func(string) error
	walk = func(dir string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		f, err := root.Open(dir)
		if err != nil {
			return ErrInvalid
		}
		children, err := f.ReadDir(-1)
		f.Close()
		if err != nil {
			return ErrInvalid
		}
		sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
		for _, child := range children {
			if strings.ContainsAny(child.Name(), "\r\n\x00") {
				return ErrInvalid
			}
			if child.Name() == ".git" {
				continue
			}
			visited++
			if visited > maxFiles {
				return ErrInvalid
			}
			name := filepath.Join(dir, child.Name())
			if len(name) > 1024 {
				return ErrInvalid
			}
			info, err := root.Lstat(name)
			if err != nil {
				return ErrInvalid
			}
			if info.IsDir() {
				if destination != "" {
					if err := os.MkdirAll(filepath.Join(destination, name), 0700); err != nil {
						return ErrInvalid
					}
				}
				if err := walk(name); err != nil {
					return err
				}
				continue
			}
			if !info.Mode().IsRegular() || info.Size() > 8<<20 || len(entries) >= maxFiles {
				return ErrInvalid
			}
			file, err := root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
			if err != nil {
				return ErrInvalid
			}
			stat, err := file.Stat()
			if err != nil || !stat.Mode().IsRegular() {
				file.Close()
				return ErrInvalid
			}
			if native, ok := stat.Sys().(*syscall.Stat_t); !ok || native.Nlink != 1 {
				file.Close()
				return ErrInvalid
			}
			data, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
			file.Close()
			if err != nil || len(data) > 8<<20 {
				return ErrInvalid
			}
			total += int64(len(data))
			if total > maxBytes {
				return ErrInvalid
			}
			mode := uint32(0600)
			if stat.Mode()&0111 != 0 {
				mode = 0700
			}
			sum := sha256.Sum256(data)
			entries = append(entries, manifestFile{Name: filepath.ToSlash(name), Hash: hex.EncodeToString(sum[:]), Mode: mode})
			if destination != "" {
				path := filepath.Join(destination, name)
				if err := os.WriteFile(path, data, os.FileMode(mode)); err != nil {
					return ErrInvalid
				}
				file, err := os.Open(path)
				if err != nil {
					return ErrInvalid
				}
				err = file.Sync()
				file.Close()
				if err != nil {
					return ErrInvalid
				}
			}
		}
		if destination != "" {
			f, err := os.Open(filepath.Join(destination, dir))
			if err != nil {
				return ErrInvalid
			}
			err = f.Sync()
			f.Close()
			if err != nil {
				return ErrInvalid
			}
		}
		return nil
	}
	if err := walk("."); err != nil {
		return "", err
	}
	raw, _ := json.Marshal(entries)
	if inventory != nil {
		inventory(entries)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func private(path string) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != filepath.Clean(path) {
		return ErrInvalid
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return ErrInvalid
	}
	return nil
}
func lock(ctx context.Context, path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, ErrInvalid
	}
	f := os.NewFile(uintptr(fd), "code workspace lock")
	for {
		if unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB) == nil {
			return f, nil
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// Prepare publishes both baseline and working copy together in an immutable
// Task location. A crash before rename leaves no admitted workspace; after it,
// the receipt is sufficient to recover the same directory without reading source.
func Prepare(ctx context.Context, root, source, taskID, scope string) (Workspace, error) {
	hash := sha256.Sum256([]byte(taskID))
	name := hex.EncodeToString(hash[:16])
	if err := os.MkdirAll(root, 0700); err != nil {
		return Workspace{}, ErrInvalid
	}
	if err := private(root); err != nil {
		return Workspace{}, err
	}
	root = filepath.Join(root, ".tasks")
	if err := os.MkdirAll(root, 0700); err != nil {
		return Workspace{}, ErrInvalid
	}
	if err := private(root); err != nil {
		return Workspace{}, err
	}
	guard, err := lock(ctx, filepath.Join(root, "."+name+".lock"))
	if err != nil {
		return Workspace{}, err
	}
	defer guard.Close()
	base := filepath.Join(root, name)
	out := Workspace{Directory: filepath.Join(base, "work"), Baseline: filepath.Join(base, "baseline"), Status: "ready"}
	read := func() error {
		if err := private(base); err != nil {
			return err
		}
		f, err := os.OpenFile(filepath.Join(base, "receipt.json"), os.O_RDONLY|unix.O_NOFOLLOW, 0)
		if err != nil {
			return ErrInvalid
		}
		defer f.Close()
		var r receipt
		if json.NewDecoder(io.LimitReader(f, 4096)).Decode(&r) != nil || r.Version != 1 || r.Scope != scope {
			return ErrInvalid
		}
		out.BaselineHash = r.BaselineHash
		actual, err := Tree(ctx, out.Baseline, "")
		if err != nil || actual != r.BaselineHash {
			return ErrInvalid
		}
		return private(out.Directory)
	}
	if _, err := os.Lstat(base); err == nil {
		return out, read()
	} else if !os.IsNotExist(err) {
		return Workspace{}, ErrInvalid
	}
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil || resolved != filepath.Clean(source) {
		return Workspace{}, ErrInvalid
	}
	// The marker may be a directory or worktree file, but its bytes/config,
	// hooks, remotes and object databases are never opened.
	marker, err := os.Lstat(filepath.Join(source, ".git"))
	if err != nil || !(marker.IsDir() || marker.Mode().IsRegular()) {
		return Workspace{}, ErrInvalid
	}
	prefix := ".prepare-" + name + "-"
	stale, err := os.ReadDir(root)
	if err != nil {
		return Workspace{}, ErrInvalid
	}
	for _, entry := range stale {
		if strings.HasPrefix(entry.Name(), prefix) {
			if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
				return Workspace{}, ErrInvalid
			}
		}
	}
	temp, err := os.MkdirTemp(root, prefix)
	if err != nil {
		return Workspace{}, ErrInvalid
	}
	defer os.RemoveAll(temp)
	for _, dir := range []string{"baseline", "work"} {
		if err := os.Mkdir(filepath.Join(temp, dir), 0700); err != nil {
			return Workspace{}, ErrInvalid
		}
	}
	baseline, err := Tree(ctx, source, filepath.Join(temp, "baseline"))
	if err != nil {
		return Workspace{}, err
	}
	if current, err := Tree(ctx, source, ""); err != nil || current != baseline {
		return Workspace{}, ErrInvalid
	}
	copyHash, err := Tree(ctx, filepath.Join(temp, "baseline"), filepath.Join(temp, "work"))
	if err != nil || copyHash != baseline {
		return Workspace{}, ErrInvalid
	}
	raw, _ := json.Marshal(receipt{Version: 1, Scope: scope, BaselineHash: baseline})
	if err := os.WriteFile(filepath.Join(temp, "receipt.json"), raw, 0600); err != nil {
		return Workspace{}, ErrInvalid
	}
	file, err := os.Open(filepath.Join(temp, "receipt.json"))
	if err != nil {
		return Workspace{}, ErrInvalid
	}
	err = file.Sync()
	file.Close()
	if err != nil {
		return Workspace{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return Workspace{}, err
	}
	if err := os.Rename(temp, base); err != nil {
		return Workspace{}, ErrInvalid
	}
	dir, err := os.Open(root)
	if err != nil {
		return Workspace{}, ErrInvalid
	}
	err = dir.Sync()
	dir.Close()
	if err != nil {
		return Workspace{}, ErrInvalid
	}
	out.BaselineHash = baseline
	return out, nil
}

type Diff struct {
	Patch             string       `json:"patch"`
	Changed           bool         `json:"changed"`
	ExecutableChanges []ModeChange `json:"executable_changes"`
}
type ModeChange struct {
	Path   string `json:"path"`
	Before bool   `json:"before_executable"`
	After  bool   `json:"after_executable"`
}

func Validate(ctx context.Context, w Workspace) error {
	if err := private(w.Directory); err != nil {
		return err
	}
	actual, err := Tree(ctx, w.Baseline, "")
	if err != nil || actual != w.BaselineHash {
		return ErrInvalid
	}
	return nil
}

// Review rejects unsafe work-tree entries before a fixed read-only recursive
// diff in the existing kernel sandbox. It never executes repository commands.
func Review(ctx context.Context, w Workspace) (Diff, error) {
	fd, err := unix.Open(w.Directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return Diff{}, ErrInvalid
	}
	guard := os.NewFile(uintptr(fd), "code review lock")
	defer guard.Close()
	if unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB) != nil {
		return Diff{}, ErrInvalid
	}
	var before, after []manifestFile
	if actual, err := scanTree(ctx, w.Baseline, "", func(entries []manifestFile) { before = entries }); err != nil || actual != w.BaselineHash {
		return Diff{}, ErrInvalid
	}
	if _, err := scanTree(ctx, w.Directory, "", func(entries []manifestFile) { after = entries }); err != nil {
		return Diff{}, ErrInvalid
	}
	temp, err := os.MkdirTemp("", "steward-code-review-")
	if err != nil {
		return Diff{}, ErrInvalid
	}
	defer os.RemoveAll(temp)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd, err := sandbox.Command(ctx, "/usr/bin/diff", []string{"-ruN", "-x", ".git", w.Baseline, w.Directory}, []string{w.Baseline, w.Directory}, temp, temp)
	if err != nil {
		return Diff{}, ErrInvalid
	}
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + temp, "LANG=C.UTF-8"}
	output := sandbox.NewOutput(8 << 20)
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	started, err := sandbox.Run(cmd, unix.SIGTERM)
	if !started || output.Overflow() || ctx.Err() != nil {
		return Diff{}, ErrInvalid
	}
	code := cmd.ProcessState.ExitCode()
	if err != nil && code != 1 {
		return Diff{}, ErrInvalid
	}
	lines := strings.Split(string(output.Bytes()), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "Binary files ") || strings.HasPrefix(line, "Only in ") {
			line = strings.ReplaceAll(line, w.Baseline+"/", "a/")
			line = strings.ReplaceAll(line, w.Directory+"/", "b/")
			line = strings.ReplaceAll(line, w.Baseline, "a")
			line = strings.ReplaceAll(line, w.Directory, "b")
			lines[i] = line
		}
	}
	patch := strings.Join(lines, "\n")
	if code == 1 && patch == "" {
		return Diff{}, fmt.Errorf("%w: missing diff output", ErrInvalid)
	}
	modes := map[string]ModeChange{}
	for _, entry := range before {
		modes[entry.Name] = ModeChange{Path: entry.Name, Before: entry.Mode&0100 != 0}
	}
	for _, entry := range after {
		change := modes[entry.Name]
		change.Path = entry.Name
		change.After = entry.Mode&0100 != 0
		modes[entry.Name] = change
	}
	changes := []ModeChange{}
	for _, change := range modes {
		if change.Before != change.After {
			changes = append(changes, change)
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return Diff{Patch: patch, Changed: code == 1 || len(changes) > 0, ExecutableChanges: changes}, nil
}
