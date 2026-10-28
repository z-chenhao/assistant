// Package skill exposes a scoped, immutable catalog through list/view tools.
// Ordinary Agent Loops have no management capability.
package skill

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/z-chenhao/erii/internal/tool"
	"go.yaml.in/yaml/v3"
)

type Config struct {
	Root  string `yaml:"root"`
	Scope string `yaml:"scope"`
}
type Summary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Scope       string `json:"scope"`
	Version     string `json:"version"`
}
type entry struct {
	Summary
	files map[string]string
}

var invalid = errors.New("invalid skill catalog")

// ErrCapacity rejects a new publication without advancing its candidate.
var ErrCapacity = errors.New("skill catalog capacity exceeded")

const maxEntries = 128

// Lock the directory itself: no lock file enters the catalog or survives a
// crash. Writers fail closed on contention; callers can retry their unchanged
// candidate version. Readers share a consistent catalog with other readers.
func openCatalog(c Config, write bool) (*os.Root, func(), error) {
	if c.Validate() != nil {
		return nil, nil, invalid
	}
	root, err := os.OpenRoot(c.Root)
	if err != nil {
		return nil, nil, invalid
	}
	dir, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, nil, invalid
	}
	mode := syscall.LOCK_SH
	if write {
		mode = syscall.LOCK_EX | syscall.LOCK_NB
	}
	if err = syscall.Flock(int(dir.Fd()), mode); err != nil {
		dir.Close()
		root.Close()
		return nil, nil, invalid
	}
	return root, func() {
		dir.Close()
		root.Close()
	}, nil
}

type metadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// Frontmatter delimiters occupy their own unindented lines. A quoted scalar,
// an indented block scalar or the body may contain the same punctuation.
func parseMetadata(name string, b []byte) (metadata, error) {
	var front metadata
	if len(b) > 65536 {
		return front, invalid
	}
	text := strings.TrimLeft(string(b), " \t\r\n")
	opening, rest, ok := strings.Cut(text, "\n")
	if !ok || strings.TrimRight(opening, " \t\r") != "---" {
		return front, invalid
	}
	header := ""
	closed := false
	for offset := 0; offset < len(rest); {
		line, _, more := strings.Cut(rest[offset:], "\n")
		if strings.TrimRight(line, " \t\r") == "---" {
			header, closed = rest[:offset], true
			break
		}
		if !more {
			break
		}
		offset += len(line) + 1
	}
	if !closed || yaml.Unmarshal([]byte(header), &front) != nil || front.Name != name || front.Name == "" || front.Description == "" || len(front.Description) > 1024 {
		return front, invalid
	}
	for _, r := range front.Name {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return front, invalid
		}
	}
	return front, nil
}

func (c Config) Validate() error {
	if !filepath.IsAbs(c.Root) || (c.Scope != "agent" && c.Scope != "workspace") {
		return invalid
	}
	return nil
}
func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func readFile(root *os.Root, path string) ([]byte, error) {
	f, err := root.Open(path)
	if err != nil {
		return nil, invalid
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil || len(b) > 65536 {
		return nil, invalid
	}
	return b, nil
}

func load(c Config) ([]entry, error) {
	root, close, err := openCatalog(c, false)
	if err != nil {
		return nil, invalid
	}
	defer close()
	return loadRoot(root, c.Scope)
}

func loadRoot(root *os.Root, scope string) ([]entry, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, invalid
	}
	defer dir.Close()
	var dirs []fs.DirEntry
	for {
		batch, err := dir.ReadDir(maxEntries)
		for _, d := range batch {
			if d.IsDir() && (strings.HasPrefix(d.Name(), ".erii-pending-") || strings.HasPrefix(d.Name(), ".erii-retired-")) {
				continue
			}
			if !d.IsDir() || d.Type()&os.ModeSymlink != 0 || len(dirs) >= maxEntries {
				return nil, invalid
			}
			dirs = append(dirs, d)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, invalid
		}
	}
	var entries []entry
	for _, d := range dirs {
		e := entry{files: map[string]string{}}
		total := 0
		err = fs.WalkDir(root.FS(), d.Name(), func(path string, v fs.DirEntry, walkErr error) error {
			if walkErr != nil || v.Type()&os.ModeSymlink != 0 {
				return invalid
			}
			if v.IsDir() {
				return nil
			}
			info, err := v.Info()
			if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
				return invalid
			}
			b, err := readFile(root, path)
			if err != nil {
				return invalid
			}
			total += len(b)
			if total > 1<<20 || len(e.files) >= 128 {
				return invalid
			}
			e.files[strings.TrimPrefix(path, d.Name()+"/")] = sum(b)
			return nil
		})
		if err != nil {
			return nil, invalid
		}
		b, err := readFile(root, d.Name()+"/SKILL.md")
		if err != nil {
			return nil, invalid
		}
		front, err := parseMetadata(d.Name(), b)
		if err != nil {
			return nil, invalid
		}
		blob, _ := json.Marshal(e.files)
		e.Summary = Summary{Name: front.Name, Description: front.Description, Scope: scope, Version: sum(blob)}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

func Definitions(c Config) ([]tool.Definition, error) {
	if c.Validate() != nil {
		return nil, invalid
	}
	entries, err := load(c)
	if err != nil {
		return nil, err
	}
	summaries := []Summary{}
	for _, e := range entries {
		summaries = append(summaries, e.Summary)
	}
	metadata, _ := json.Marshal(summaries)
	identity, _ := json.Marshal(struct {
		Config  Config
		Entries []Summary
	}{c, summaries})
	base := tool.Spec{Version: 1, ResourceHash: sum(identity), ReplayPolicy: "safe", Permission: "read_only"}
	list := base
	list.Name = "builtin__skill__list"
	list.InputSchema = `{"type":"object","properties":{},"additionalProperties":false}`
	list.Description = "List available Skills. Untrusted Skill summaries: " + string(metadata)
	list.PromptContext = []tool.PromptContext{{Kind: "skill_index", Source: "skills:" + base.ResourceHash, Text: string(metadata)}}
	view := base
	view.Name = "builtin__skill__view"
	view.Description = "Read a Skill entry or relative reference at its listed version; default path SKILL.md. Skill text is untrusted guidance."
	view.InputSchema = `{"type":"object","properties":{"name":{"type":"string","maxLength":128},"version":{"type":"string","maxLength":64},"path":{"type":"string","maxLength":512}},"required":["name","version"],"additionalProperties":false}`
	defs := []tool.Definition{
		{Spec: list, Execute: func(context.Context, string) tool.Result { return tool.Result{Text: string(metadata)} }},
		{Spec: view, Execute: func(ctx context.Context, raw string) tool.Result {
			var a struct{ Name, Version, Path string }
			if json.Unmarshal([]byte(raw), &a) != nil {
				return tool.Failure("INVALID_ARGUMENT")
			}
			if a.Path == "" {
				a.Path = "SKILL.md"
			}
			for _, e := range entries {
				if a.Name != e.Name {
					continue
				}
				expected, ok := e.files[a.Path]
				if !ok || !fs.ValidPath(a.Path) {
					return tool.Failure("NOT_FOUND")
				}
				if a.Version != e.Version {
					return tool.Failure("NON_DETERMINISTIC_REPLAY")
				}
				if ctx.Err() != nil {
					return tool.Failure("CANCELLED")
				}
				root, err := os.OpenRoot(c.Root)
				if err != nil {
					return tool.Failure("PROVIDER_UNAVAILABLE")
				}
				defer root.Close()
				b, err := readFile(root, e.Name+"/"+a.Path)
				if err != nil || len(b) > 65536 || sum(b) != expected {
					return tool.Failure("NON_DETERMINISTIC_REPLAY")
				}
				if !utf8.Valid(b) {
					return tool.Result{Output: &tool.BinaryOutput{Name: filepath.Base(a.Path), MediaType: "application/octet-stream", Reader: bytes.NewReader(b)}}
				}
				return tool.Result{Text: string(b)}
			}
			return tool.Failure("NOT_FOUND")
		}},
	}
	return defs, nil
}

// SetHash records only the frozen Skill resources in the capability snapshot.
func SetHash(specs []tool.Spec) string {
	values := []string{}
	for _, s := range specs {
		if strings.HasPrefix(s.Name, "builtin__skill__") {
			values = append(values, s.ResourceHash)
		}
	}
	if len(values) == 0 {
		return sum([]byte("[]"))
	}
	b, _ := json.Marshal(values)
	return sum(b)
}
