package coding

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func paths(t *testing.T) (string, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, ".git", "hooks"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "notes.txt"), []byte("owner dirty content\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".git", "config"), []byte("CREDENTIAL_METADATA_MUST_NOT_BE_COPIED"), 0600); err != nil {
		t.Fatal(err)
	}
	return source, filepath.Join(root, "execution")
}
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestIndependentWorkingCopiesRetainDirtyBaselineAndRecover(t *testing.T) {
	source, root := paths(t)
	ctx := context.Background()
	a, err := Prepare(ctx, root, source, "a", "frozen")
	must(t, err)
	b, err := Prepare(ctx, root, source, "b", "frozen")
	must(t, err)
	if a.Directory == b.Directory || a.BaselineHash != b.BaselineHash {
		t.Fatal(a, b)
	}
	must(t, os.WriteFile(filepath.Join(a.Directory, "notes.txt"), []byte("Task A edit\n"), 0600))
	for _, dir := range []string{source, b.Directory, a.Baseline} {
		raw, err := os.ReadFile(filepath.Join(dir, "notes.txt"))
		must(t, err)
		if string(raw) != "owner dirty content\n" {
			t.Fatal("work polluted source or peer", dir)
		}
	}
	if _, err := os.Stat(filepath.Join(a.Directory, ".git")); !os.IsNotExist(err) {
		t.Fatal("source Git metadata imported")
	}
	must(t, os.WriteFile(filepath.Join(source, "notes.txt"), []byte("later owner edit\n"), 0600))
	again, err := Prepare(ctx, root, source, "a", "frozen")
	must(t, err)
	if again != a {
		t.Fatal("restart rebound snapshot", again, a)
	}
	if _, err := Prepare(ctx, root, source, "a", "changed-scope"); err == nil {
		t.Fatal("changed frozen scope accepted")
	}
}

func TestUnsafeSourceAndChangedBaselineFailClosed(t *testing.T) {
	source, root := paths(t)
	ctx := context.Background()
	outside := filepath.Join(filepath.Dir(source), "private")
	must(t, os.WriteFile(outside, []byte("private"), 0600))
	must(t, os.Symlink(outside, filepath.Join(source, "link")))
	if _, err := Prepare(ctx, root, source, "bad", "scope"); err == nil {
		t.Fatal("source symlink imported")
	}
	must(t, os.Remove(filepath.Join(source, "link")))
	must(t, os.Link(outside, filepath.Join(source, "hardlink")))
	if _, err := Prepare(ctx, root, source, "bad", "scope"); err == nil {
		t.Fatal("source hardlink imported")
	}
	must(t, os.Remove(filepath.Join(source, "hardlink")))
	w, err := Prepare(ctx, root, source, "bad", "scope")
	must(t, err)
	must(t, os.WriteFile(filepath.Join(w.Baseline, "notes.txt"), []byte("changed baseline"), 0600))
	if _, err := Prepare(ctx, root, source, "bad", "scope"); err == nil {
		t.Fatal("tampered baseline reused")
	}
	if _, err := Review(ctx, w); err == nil {
		t.Fatal("tampered baseline reported no differences")
	}
}

func TestSandboxReviewProducesRelativePatchAndRefusesLinks(t *testing.T) {
	source, root := paths(t)
	ctx := context.Background()
	w, err := Prepare(ctx, root, source, "review", "scope")
	must(t, err)
	must(t, os.WriteFile(filepath.Join(w.Directory, "notes.txt"), []byte("Task edit\n"), 0700))
	must(t, os.Chmod(filepath.Join(w.Directory, "notes.txt"), 0700))
	must(t, os.WriteFile(filepath.Join(w.Directory, "new.txt"), []byte("new file\n"), 0600))
	diff, err := Review(ctx, w)
	must(t, err)
	if !diff.Changed || !strings.Contains(diff.Patch, "--- a/notes.txt") || !strings.Contains(diff.Patch, "+++ b/notes.txt") || strings.Contains(diff.Patch, root) || len(diff.ExecutableChanges) != 1 {
		t.Fatal(diff)
	}
	must(t, os.Symlink("/etc/passwd", filepath.Join(w.Directory, "escape")))
	if _, err := Review(ctx, w); err == nil {
		t.Fatal("unsafe changed tree reviewed as ordinary diff")
	}
}
