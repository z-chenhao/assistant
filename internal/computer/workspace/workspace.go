// Package workspace is the M3 controlled coding boundary. The operator supplies
// an exclusive disposable Git checkout, never the user's live working tree.
package workspace

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/z-chenhao/erii/internal/tool"
)

type Config struct {
	Root      string   `yaml:"root"`
	Mode      string   `yaml:"mode"` // audit or edit
	Protected []string `yaml:"protected"`
}

var invalid = errors.New("invalid isolated coding workspace")

// Safe Host doctor categories. Never include a provider response or host path.
func diagnosis(phase string) error { return fmt.Errorf("%w: %s", invalid, phase) }

func (c Config) Validate() error {
	if !filepath.IsAbs(c.Root) || (c.Mode != "audit" && c.Mode != "edit") || len(c.Protected) > 128 {
		return invalid
	}
	for _, p := range c.Protected {
		if !fs.ValidPath(p) || strings.HasPrefix(p, ".git") {
			return invalid
		}
	}
	return nil
}
func hash(b []byte) string     { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func result(v any) tool.Result { b, _ := json.Marshal(v); return tool.Result{Text: string(b)} }

type Document struct {
	Path string `json:"path"`
	Text string `json:"text"`
	Hash string `json:"hash"`
}

// Only regular, singly-linked files under os.Root are visible. Reject symlinks
// even when their target happens to remain inside the checkout.
func regular(root *os.Root, path string) bool {
	if !fs.ValidPath(path) || path == "." {
		return false
	}
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == ".git" || strings.HasPrefix(p, ".erii-") {
			return false
		}
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if i == len(parts)-1 {
			stat, ok := info.Sys().(*syscall.Stat_t)
			return info.Mode().IsRegular() && info.Size() <= 65536 && ok && stat.Nlink == 1
		}
		if !info.IsDir() {
			return false
		}
	}
	return false
}
func read(root *os.Root, path string) (Document, error) {
	if !regular(root, path) {
		return Document{}, invalid
	}
	f, err := root.Open(path)
	if err != nil {
		return Document{}, invalid
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil || len(b) > 65536 {
		return Document{}, invalid
	}
	return Document{Path: path, Text: string(b), Hash: hash(b)}, nil
}

// These fixed read-only Git commands disable hooks, textconv, external diff,
// fsmonitor, user config, pagers, replacement objects and optional index writes.
func git(ctx context.Context, c Config, args ...string) ([]byte, error) {
	// Never load repository config: filters and fsmonitor can execute programs
	// even during status/diff. Use a bounded metadata copy without config/hooks.
	metadata, err := os.MkdirTemp("", "erii-git-")
	if err != nil {
		return nil, invalid
	}
	defer os.RemoveAll(metadata)
	source, err := os.OpenRoot(filepath.Join(c.Root, ".git"))
	if err != nil {
		return nil, invalid
	}
	defer source.Close()
	count, total := 0, int64(0)
	err = fs.WalkDir(source.FS(), ".", func(p string, d fs.DirEntry, e error) error {
		if e != nil || ctx.Err() != nil {
			return diagnosis("metadata_walk")
		}
		count++
		if count > 4096 {
			return invalid
		}
		// Locks are transient coordination state, not repository contents.
		// In particular, detached Git maintenance may remove one between
		// enumeration and ReadFile. Never copy or follow it into the snapshot.
		if strings.HasSuffix(p, ".lock") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p == "hooks" || p == "logs" {
				return fs.SkipDir
			}
			if p == "." {
				return nil
			}
			if os.MkdirAll(filepath.Join(metadata, p), 0700) != nil {
				return diagnosis("metadata_copy")
			}
			return nil
		}
		if p == "config" || p == "config.worktree" || p == "info/attributes" {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || p == "commondir" || p == "objects/info/alternates" {
			return diagnosis("metadata_link")
		}
		info, e := d.Info()
		if e != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			return diagnosis("metadata_file")
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Nlink != 1 {
			return diagnosis("metadata_hardlink")
		}
		total += info.Size()
		if total > 8<<20 {
			return invalid
		}
		b, e := source.ReadFile(p)
		if e != nil {
			return diagnosis("metadata_read")
		}
		if os.WriteFile(filepath.Join(metadata, p), b, 0600) != nil {
			return diagnosis("metadata_copy")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	bin, err := exec.LookPath("git")
	if err != nil {
		return nil, invalid
	}
	prefix := []string{"--no-optional-locks", "-c", "maintenance.auto=false", "-c", "gc.auto=0", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-c", "core.pager=cat", "-c", "core.attributesFile=/dev/null", "-c", "diff.ignoreSubmodules=all", "-c", "core.worktree=" + c.Root}
	cmd := exec.CommandContext(ctx, bin, append(prefix, args...)...)
	cmd.Dir = c.Root
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=/nonexistent", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_ATTR_NOSYSTEM=1", "GIT_NO_REPLACE_OBJECTS=1", "GIT_DIR=" + metadata, "GIT_WORK_TREE=" + c.Root}
	cmd.WaitDelay = time.Second
	out := &limitedBuffer{limit: 65536}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	if cmd.Run() != nil || out.overflow {
		return nil, diagnosis("git_command")
	}
	return out.data, nil
}

func Definitions(c Config) ([]tool.Definition, error) {
	if c.Validate() != nil {
		return nil, invalid
	}
	canonical, err := filepath.EvalSymlinks(c.Root)
	if err != nil {
		return nil, invalid
	}
	c.Root = canonical
	root, err := os.OpenRoot(c.Root)
	if err != nil {
		return nil, invalid
	}
	defer root.Close()
	rules, err := read(root, "AGENTS.md")
	if err != nil {
		return nil, invalid
	}
	info, err := root.Lstat(".git")
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, invalid
	}
	// Worktrees and alternates can escape the configured checkout.
	if _, err = root.Stat(".git/commondir"); !os.IsNotExist(err) {
		return nil, invalid
	}
	if _, err = root.Stat(".git/objects/info/alternates"); !os.IsNotExist(err) {
		return nil, invalid
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err = git(ctx, c, "status", "--ignore-submodules=all", "--porcelain=v1", "-z", "--untracked-files=all"); err != nil {
		return nil, err
	}
	identity, _ := json.Marshal(c)
	base := tool.Spec{Version: 1, ResourceHash: hash(identity), ReplayPolicy: "safe", Permission: "read_only", Provider: "workspace:" + hash([]byte(c.Root)), Risk: "low", TimeoutMS: 300000, OutputBytes: 65536, ConflictPolicy: "workspace-v1"}
	makeDef := func(name, schema, description string, fn func(context.Context, string) tool.Result) tool.Definition {
		s := base
		s.Name = "builtin__workspace__" + name
		s.InputSchema = schema
		s.Description = description
		if name == "read" {
			s.PromptContext = []tool.PromptContext{{Kind: "project_instructions", Source: "workspace:AGENTS.md:" + rules.Hash, Text: rules.Text}}
		}
		return tool.Definition{Spec: s, Execute: fn, ConflictKeys: func(string) []string { return []string{"workspace:" + hash([]byte(c.Root))} }}
	}
	open := func() (*os.Root, error) { return os.OpenRoot(c.Root) }
	defs := []tool.Definition{
		makeDef("read", `{"type":"object","properties":{"path":{"type":"string","maxLength":512}},"required":["path"],"additionalProperties":false}`, "Read a relative workspace file and its content hash. Read AGENTS.md first.", func(ctx context.Context, raw string) tool.Result {
			var a struct{ Path string }
			_ = json.Unmarshal([]byte(raw), &a)
			r, err := open()
			if err != nil {
				return tool.Failure("PROVIDER_UNAVAILABLE")
			}
			defer r.Close()
			d, err := read(r, a.Path)
			if err != nil {
				return tool.Failure("NOT_FOUND")
			}
			return result(d)
		}),
		makeDef("search", `{"type":"object","properties":{"query":{"type":"string","maxLength":256}},"required":["query"],"additionalProperties":false}`, "Find literal text in regular workspace files; empty query lists files. Results are bounded.", func(ctx context.Context, raw string) tool.Result {
			var a struct{ Query string }
			_ = json.Unmarshal([]byte(raw), &a)
			r, err := open()
			if err != nil {
				return tool.Failure("PROVIDER_UNAVAILABLE")
			}
			defer r.Close()
			found := []Document{}
			count := 0
			err = fs.WalkDir(r.FS(), ".", func(p string, d fs.DirEntry, e error) error {
				if e != nil {
					return invalid
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				count++
				if count > 4096 {
					return invalid
				}
				if d.IsDir() {
					if d.Name() == ".git" || strings.HasPrefix(d.Name(), ".erii-") {
						return fs.SkipDir
					}
					return nil
				}
				doc, e := read(r, p)
				if e != nil {
					return nil
				}
				if a.Query == "" || strings.Contains(doc.Text, a.Query) {
					doc.Text = ""
					found = append(found, doc)
				}
				if len(found) > 128 {
					return invalid
				}
				return nil
			})
			if err != nil {
				return tool.Failure("RESOURCE_LIMIT")
			}
			return result(found)
		}),
		makeDef("git", `{"type":"object","properties":{"operation":{"enum":["status","diff"]}},"required":["operation"],"additionalProperties":false}`, "Read Git status or diff; no mutating Git commands or hook bypass capability.", func(ctx context.Context, raw string) tool.Result {
			var a struct{ Operation string }
			_ = json.Unmarshal([]byte(raw), &a)
			args := []string{"status", "--ignore-submodules=all", "--porcelain=v1", "--untracked-files=all"}
			if a.Operation == "diff" {
				args = []string{"diff", "--no-ext-diff", "--no-textconv", "--"}
			}
			b, err := git(ctx, c, args...)
			if err != nil {
				return tool.Failure("PROVIDER_UNAVAILABLE")
			}
			return tool.Result{Text: string(b)}
		}),
		makeDef("test", `{"type":"object","properties":{},"additionalProperties":false}`, "Run go test -race ./... on a disposable isolated copy. Network and host user files are unavailable. Failure is reported as passed:false.", func(ctx context.Context, raw string) tool.Result {
			r, err := open()
			if err != nil {
				return tool.Failure("PROVIDER_UNAVAILABLE")
			}
			defer r.Close()
			return test(ctx, r)
		}),
	}
	if c.Mode == "edit" {
		d := makeDef("edit", `{"type":"object","properties":{"path":{"type":"string","maxLength":512},"before_hash":{"type":"string","maxLength":64},"rules_hash":{"type":"string","maxLength":64},"text":{"type":"string","maxLength":65536}},"required":["path","before_hash","rules_hash","text"],"additionalProperties":false}`, "Replace one clean tracked file after reading AGENTS.md; requires matching preimage and rules hashes. Protected and dirty files are denied. Exact action requires approval.", nil)
		d.Spec.Permission = "external_effect"
		d.Spec.Risk = "high"
		d.Spec.ReplayPolicy = "never"
		d.Invoke = func(ctx context.Context, request tool.Request) tool.Outcome {
			fail := func(code string) tool.Outcome { return tool.Outcome{Result: tool.Failure(code)} }
			var a struct {
				Path   string `json:"path"`
				Before string `json:"before_hash"`
				Rules  string `json:"rules_hash"`
				Text   string `json:"text"`
			}
			if json.Unmarshal([]byte(request.Arguments), &a) != nil {
				return fail("INVALID_ARGUMENT")
			}
			if a.Path == "AGENTS.md" {
				return fail("PERMISSION_DENIED")
			}
			for _, p := range c.Protected {
				if a.Path == p || strings.HasPrefix(a.Path, p+"/") {
					return fail("PERMISSION_DENIED")
				}
			}
			r, err := open()
			if err != nil {
				return fail("PROVIDER_UNAVAILABLE")
			}
			defer r.Close()
			doc, err := read(r, a.Path)
			if err != nil {
				return fail("PERMISSION_DENIED")
			}
			rules, err := read(r, "AGENTS.md")
			if err != nil || rules.Hash != a.Rules || doc.Hash != a.Before {
				return fail("NON_DETERMINISTIC_REPLAY")
			}
			// The fixed literal pathspec prevents names from becoming Git options.
			// Status trusts assume-unchanged/skip-worktree flags. Accept only
			// one ordinary cached entry: -v lowers its tag for assume-unchanged
			// and uses S for skip-worktree. Inspect the private index copy;
			// never clear flags or refresh the user's original index.
			entry, err := git(ctx, c, "ls-files", "-v", "-z", "--error-unmatch", "--", ":(literal)"+a.Path)
			if err != nil || string(entry) != "H "+a.Path+"\x00" {
				return fail("PERMISSION_DENIED")
			}
			dirty, err := git(ctx, c, "status", "--ignore-submodules=all", "--porcelain=v1", "-z", "--untracked-files=all", "--", ":(literal)"+a.Path)
			if err != nil {
				return fail("PROVIDER_UNAVAILABLE")
			}
			if len(dirty) != 0 {
				return fail("PERMISSION_DENIED")
			}
			if ctx.Err() != nil {
				return fail("CANCELLED")
			}
			// Create beside the target and atomically publish only a complete file.
			// Scratch names must not disclose the invocation/provider key if a
			// crash leaves an untracked file visible in Git status.
			tmp := filepath.ToSlash(filepath.Join(filepath.Dir(a.Path), ".erii-edit-"+rand.Text()))
			f, err := r.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				return fail("PROVIDER_UNAVAILABLE")
			}
			defer r.Remove(tmp)
			_, err = io.WriteString(f, a.Text)
			if err == nil {
				err = f.Sync()
			}
			closeErr := f.Close()
			latest, e := read(r, a.Path)
			if err != nil || closeErr != nil {
				return fail("PROVIDER_UNAVAILABLE")
			}
			if e != nil || latest.Hash != a.Before {
				return fail("NON_DETERMINISTIC_REPLAY")
			}
			info, err := r.Stat(a.Path)
			if err != nil || r.Chmod(tmp, info.Mode().Perm()) != nil {
				return fail("PROVIDER_UNAVAILABLE")
			}
			if ctx.Err() != nil {
				return fail("CANCELLED")
			}
			if err = r.Rename(tmp, a.Path); err != nil {
				return fail("PROVIDER_UNAVAILABLE")
			}
			return tool.Outcome{Result: result(Document{Path: a.Path, Hash: hash([]byte(a.Text))})}
		}
		defs = append(defs, d)
	}
	return defs, nil
}
