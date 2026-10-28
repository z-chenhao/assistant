package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/tool"
)

func TestEditRejectsHiddenIndexEntriesWithoutChangingUserState(t *testing.T) {
	for _, flag := range []string{"", "--assume-unchanged", "--skip-worktree"} {
		for _, dirty := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dirty=%t", flag, dirty), func(t *testing.T) {
				c := fixture(t)
				path := "clean.go"
				if flag != "" {
					cmd := exec.Command("git", "update-index", flag, "--", path)
					cmd.Dir = c.Root
					cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=/nonexistent", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}
					if out, err := cmd.CombinedOutput(); err != nil {
						t.Fatal(string(out), err)
					}
				}
				if dirty {
					if err := os.WriteFile(filepath.Join(c.Root, path), []byte("package fixture\n// USER_UNCOMMITTED_CHANGE\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				before, err := os.ReadFile(filepath.Join(c.Root, path))
				if err != nil {
					t.Fatal(err)
				}
				index, err := os.ReadFile(filepath.Join(c.Root, ".git/index"))
				if err != nil {
					t.Fatal(err)
				}
				rules, err := os.ReadFile(filepath.Join(c.Root, "AGENTS.md"))
				if err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(map[string]string{"path": path, "before_hash": hash(before), "rules_hash": hash(rules), "text": "package replacement\n"})
				outcome := definition(t, c, "edit").Invoke(context.Background(), tool.Request{Arguments: string(raw)})
				wantDenied := dirty || flag != ""
				if wantDenied && (outcome.Ambiguous || outcome.Result.Error == nil || outcome.Result.Error.Code != "PERMISSION_DENIED") {
					t.Fatalf("expected refusal, got %+v", outcome)
				}
				if !wantDenied && outcome.Result.Error != nil {
					t.Fatal(outcome)
				}
				after, err := os.ReadFile(filepath.Join(c.Root, path))
				if err != nil || (wantDenied && !bytes.Equal(after, before)) {
					t.Fatal("user content changed", err)
				}
				afterIndex, err := os.ReadFile(filepath.Join(c.Root, ".git/index"))
				if err != nil || !bytes.Equal(afterIndex, index) {
					t.Fatal("original index changed", err)
				}
			})
		}
	}
}

func fixture(t *testing.T) Config {
	t.Helper()
	root := t.TempDir()
	for p, text := range map[string]string{"AGENTS.md": "Protect sentinel.txt. Run tests.", "clean.go": "package fixture\n", "sentinel.txt": "original\n", "go.mod": "module fixture\n\ngo 1.26.2\n"} {
		if err := os.WriteFile(filepath.Join(root, p), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "fixture"}} {
		cmd := exec.Command("git", append([]string{"-c", "maintenance.auto=false", "-c", "gc.auto=0"}, args...)...)
		cmd.Dir = root
		cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=/nonexistent", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(string(out), err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "sentinel.txt"), []byte("USER_UNCOMMITTED_SENTINEL"), 0600); err != nil {
		t.Fatal(err)
	}
	return Config{Root: root, Mode: "edit", Protected: []string{"sentinel.txt"}}
}
func definition(t *testing.T, c Config, name string) tool.Definition {
	t.Helper()
	defs, err := Definitions(c)
	if err != nil {
		// Only disposable fixture metadata is logged. This remains available
		// in Linux CI when a platform-specific doctor check rejects a fixture.
		_ = filepath.WalkDir(filepath.Join(c.Root, ".git"), func(p string, d os.DirEntry, e error) error {
			if e == nil && !d.IsDir() {
				info, statErr := d.Info()
				if statErr == nil {
					relative, _ := filepath.Rel(c.Root, p)
					t.Logf("fixture metadata %s mode=%s stat=%+v", relative, info.Mode(), info.Sys())
				}
			}
			return nil
		})
		t.Fatal(err)
	}
	r, err := tool.NewRegistry(defs)
	if err != nil {
		t.Fatal(err)
	}
	d, ok := r.Resolve("builtin__workspace_" + name)
	if !ok {
		t.Fatal(name)
	}
	return d
}
func TestEditProtectsDirtyFilesAndPreimages(t *testing.T) {
	c := fixture(t)
	d := definition(t, c, "edit")
	ctx := context.Background()
	invoke := func(path, before, rules, text string) tool.Result {
		raw, _ := json.Marshal(map[string]string{"path": path, "before_hash": before, "rules_hash": rules, "text": text})
		return d.Invoke(ctx, tool.Request{Arguments: string(raw), IdempotencyKey: "host-key"}).Result
	}
	rules, _ := os.ReadFile(filepath.Join(c.Root, "AGENTS.md"))
	old, _ := os.ReadFile(filepath.Join(c.Root, "clean.go"))
	for _, p := range []string{"sentinel.txt", "AGENTS.md", "../outside", ".git/config"} {
		r := invoke(p, hash(old), hash(rules), "overwrite")
		if r.Error == nil {
			t.Fatal("edited protected", p)
		}
	}
	if r := invoke("clean.go", "stale", hash(rules), "package fixed\n"); r.Error == nil || r.Error.Code != "NON_DETERMINISTIC_REPLAY" {
		t.Fatal(r)
	}
	if r := invoke("clean.go", hash(old), "unread-rules", "package fixed\n"); r.Error == nil {
		t.Fatal(r)
	}
	if r := invoke("clean.go", hash(old), hash(rules), "package fixed\n"); r.Error != nil {
		t.Fatal(r)
	}
	if r := invoke("clean.go", hash([]byte("package fixed\n")), hash(rules), "package again\n"); r.Error == nil || r.Error.Code != "PERMISSION_DENIED" {
		t.Fatal("edited dirty file", r)
	}
	b, _ := os.ReadFile(filepath.Join(c.Root, "sentinel.txt"))
	if string(b) != "USER_UNCOMMITTED_SENTINEL" {
		t.Fatal(string(b))
	}
	c.Mode = "audit"
	defs, err := Definitions(c)
	if err != nil {
		t.Fatal(err)
	}
	reg, _ := tool.NewRegistry(defs)
	if _, ok := reg.Resolve("builtin__workspace_edit"); ok {
		t.Fatal("audit edit visible")
	}
}
func TestFilesCannotEscapeThroughLinksOrPaths(t *testing.T) {
	c := fixture(t)
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("SECRET"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(c.Root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(outside, filepath.Join(c.Root, "hardlink")); err != nil {
		t.Fatal(err)
	}
	d := definition(t, c, "read")
	for _, p := range []string{"../secret", outside, "link", "hardlink", ".git/config", "./AGENTS.md"} {
		raw, _ := json.Marshal(map[string]string{"path": p})
		r := d.Execute(context.Background(), string(raw))
		if r.Error == nil || strings.Contains(r.ModelText(), "SECRET") {
			t.Fatal(p, r)
		}
	}
	r := d.Execute(context.Background(), `{"path":"AGENTS.md"}`)
	if r.Error != nil || !strings.Contains(r.Text, "Protect") {
		t.Fatal(r)
	}
}
func TestSandboxExecutesRealCodeWithoutHostFilesOrNetwork(t *testing.T) {
	c := fixture(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	secret := filepath.Join(t.TempDir(), "host-secret")
	if err := os.WriteFile(secret, []byte("SECRET"), 0600); err != nil {
		t.Fatal(err)
	}
	code := `package fixture
import("os";"net";"testing";"time")
func TestBoundary(t *testing.T){
 if _,e:=os.ReadFile(` + "`" + secret + "`" + `);e==nil{t.Fatal("host read")}
 if e:=os.WriteFile(` + "`" + secret + "`" + `,[]byte("changed"),0600);e==nil{t.Fatal("host write")}
 if e:=os.WriteFile("sentinel.txt",[]byte("copy only"),0600);e!=nil{t.Fatal(e)}
 if os.Getenv("ERII_DATA_ROOT")!=""||os.Getenv("ERII_TEST_SECRET")!=""{t.Fatal("environment leak")}
 c,e:=net.DialTimeout("tcp",` + "`" + listener.Addr().String() + "`" + `,time.Second);if e==nil{c.Close();t.Fatal("network")}
}

`
	if err := os.WriteFile(filepath.Join(c.Root, "boundary_test.go"), []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ERII_TEST_SECRET", "SENSITIVE")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	r := definition(t, c, "test").Execute(ctx, `{}`)
	var observed struct {
		Passed bool
		Output string
	}
	if r.Error != nil || json.Unmarshal([]byte(r.Text), &observed) != nil || !observed.Passed {
		t.Fatal(r.ModelText())
	}
	b, _ := os.ReadFile(secret)
	if string(b) != "SECRET" {
		t.Fatal("host secret modified")
	}
	b, _ = os.ReadFile(filepath.Join(c.Root, "sentinel.txt"))
	if string(b) != "USER_UNCOMMITTED_SENTINEL" {
		t.Fatal("test modified source checkout")
	}
}

func TestReadOnlyGitNeverExecutesCheckoutConfig(t *testing.T) {
	c := fixture(t)
	marker := filepath.Join(c.Root, "filter-ran")
	if err := os.WriteFile(filepath.Join(c.Root, ".gitattributes"), []byte("*.go filter=evil diff=evil\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(c.Root, ".git/config"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("\n[filter \"evil\"]\n clean = touch " + marker + "\n[diff \"evil\"]\n textconv = touch " + marker + "\n[core]\n fsmonitor = touch " + marker + "\n")
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	d := definition(t, c, "git")
	for _, args := range []string{`{"operation":"status"}`, `{"operation":"diff"}`} {
		if r := d.Execute(context.Background(), args); r.Error != nil {
			t.Fatal(r)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("Git executed repository config", err)
	}
}

func TestWorkspaceMetadataSnapshotIgnoresTransientLocks(t *testing.T) {
	c := fixture(t)
	// Git's automatic maintenance owns this non-authoritative lock. Its
	// lifetime/target must not become an input to read-only status or diff.
	lock := filepath.Join(c.Root, ".git/objects/maintenance.lock")
	if err := os.Symlink(filepath.Join(t.TempDir(), "vanished"), lock); err != nil {
		t.Fatal(err)
	}
	defs, err := Definitions(c)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := tool.NewRegistry(defs)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := registry.Resolve("builtin__workspace_git")
	if got := d.Execute(context.Background(), `{"operation":"status"}`); got.Error != nil {
		t.Fatal(got)
	}
	if _, err := os.Lstat(lock); err != nil {
		t.Fatal("source lock was changed", err)
	}
}
