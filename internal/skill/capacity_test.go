package skill

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func fillCatalog(t *testing.T, c Config, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("fixture-%03d", i)
		if err := os.Mkdir(filepath.Join(c.Root, name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(c.Root, name, "SKILL.md"), content(name, "Fixture method.", "1. Read source.\n2. Run tests."), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestConcurrentCatalogPublicationAtLastSlot(t *testing.T) {
	c := Config{Root: t.TempDir(), Scope: "agent"}
	fillCatalog(t, c, 127)
	start := make(chan struct{})
	type result struct {
		name string
		err  error
	}
	done := make(chan result, 2)
	for _, name := range []string{"first", "second"} {
		go func() {
			<-start
			done <- result{name, Publish(context.Background(), c, name, name, "Test method.", "1. Read source.\n2. Test.")}
		}()
	}
	close(start)
	one, two := <-done, <-done
	if (one.err == nil) == (two.err == nil) {
		t.Fatal("last slot needs exactly one owner", one, two)
	}
	if entries, err := load(c); err != nil || len(entries) != 128 {
		t.Fatal("concurrent publication broke catalog", len(entries), err)
	}
}

func TestCatalogWriterProcess(t *testing.T) {
	root := os.Getenv("ERII_SKILL_CRASH_ROOT")
	if root == "" {
		return
	}
	r, close, err := openCatalog(Config{Root: root, Scope: "agent"}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	if err := r.Mkdir(".erii-pending-crash", 0700); err != nil {
		t.Fatal(err)
	}
	if err := r.WriteFile(".erii-pending-crash/SKILL.md", []byte("partial write"), 0600); err != nil {
		t.Fatal(err)
	}
	fmt.Println("locked")
	select {}
}

func TestCatalogWriterCrashReleasesAdmission(t *testing.T) {
	c := Config{Root: t.TempDir(), Scope: "agent"}
	fillCatalog(t, c, 127)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCatalogWriterProcess$")
	cmd.Env = append(os.Environ(), "ERII_SKILL_CRASH_ROOT="+c.Root)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	scan := bufio.NewScanner(stdout)
	if !scan.Scan() || scan.Text() != "locked" {
		t.Fatal("writer never reached crash boundary", scan.Text(), scan.Err())
	}
	if err := Publish(ctx, c, "other", "other", "Test method.", "1. Read source.\n2. Test."); err == nil {
		t.Fatal("concurrent process bypassed directory lock")
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err = cmd.Wait(); err == nil {
		t.Fatal("fixture was not killed")
	}
	if err = Publish(ctx, c, "crash", "recovered", "Test method.", "1. Read source.\n2. Test."); err != nil {
		t.Fatal("crash left admission locked or staged entry unrecoverable", err)
	}
	if entries, err := load(c); err != nil || len(entries) != 128 {
		t.Fatal("recovered catalog not readable", len(entries), err)
	}
}

func TestCatalogCapacityAdmissionAndRecovery(t *testing.T) {
	c := Config{Root: t.TempDir(), Scope: "agent"}
	fillCatalog(t, c, 127)
	ctx := context.Background()
	const description, procedure = "Review --- safely.", "1. Read source.\n2. Run tests."
	for i := 0; i < 2; i++ { // Retry after rename but before the candidate status commit.
		if err := Publish(ctx, c, "last", "last-slot", description, procedure); err != nil {
			t.Fatal("last slot or recovery rejected", err)
		}
	}
	if err := Publish(ctx, c, "excess", "excess", description, procedure); err == nil {
		t.Fatal("publication exceeded catalog capacity")
	}
	if _, err := os.Stat(filepath.Join(c.Root, "excess")); !os.IsNotExist(err) {
		t.Fatal("rejected publication left an active entry", err)
	}
	if entries, err := load(c); err != nil || len(entries) != 128 {
		t.Fatal("full catalog unreadable after rejection", len(entries), err)
	}
	if err := Retire(c, "last", "last-slot", description, procedure); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(c.Root, ".erii-pending-next")
	if err := os.Mkdir(staging, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "SKILL.md"), []byte("partial write"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Definitions(c); err != nil {
		t.Fatal("retained or pending history disabled startup", err)
	}
	if err := Publish(ctx, c, "next", "next", description, procedure); err != nil {
		t.Fatal("freed slot or staged recovery rejected", err)
	}
	if err := Retire(c, "next", "next", description, procedure); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"last", "next"} {
		if _, err := os.ReadFile(filepath.Join(c.Root, ".erii-retired-"+id, "SKILL.md")); err != nil {
			t.Fatal("audit history lost", err)
		}
	}
	if entries, err := load(c); err != nil || len(entries) != 127 {
		t.Fatal("retirement disabled startup", len(entries), err)
	}
}

func TestCatalogBoundsActiveEntriesWithoutLoadingHistory(t *testing.T) {
	c := Config{Root: t.TempDir(), Scope: "agent"}
	fillCatalog(t, c, 128)
	for i := 0; i < 260; i++ {
		for _, prefix := range []string{".erii-retired-", ".erii-pending-"} {
			path := filepath.Join(c.Root, fmt.Sprintf("%s%d", prefix, i))
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("unreadable historical metadata"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if entries, err := load(c); err != nil || len(entries) != 128 {
		t.Fatal("history consumed live catalog capacity", len(entries), err)
	}
	if err := os.Mkdir(filepath.Join(c.Root, "overflow"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Definitions(c); err == nil {
		t.Fatal("active catalog limit weakened")
	}
}

func TestPublicationChecksCatalogAndRecoverableStage(t *testing.T) {
	for _, mode := range []string{"invalid-catalog", "extra-stage-resource", "linked-stage-file"} {
		t.Run(mode, func(t *testing.T) {
			c := catalog(t)
			original := filepath.Join(c.Root, "review", "SKILL.md")
			if mode == "invalid-catalog" {
				if err := os.WriteFile(original, []byte("broken metadata"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				stage := filepath.Join(c.Root, ".erii-pending-candidate")
				if err := os.Mkdir(stage, 0700); err != nil {
					t.Fatal(err)
				}
				if mode == "extra-stage-resource" {
					if err := os.WriteFile(filepath.Join(stage, "unexpected"), []byte("retained"), 0600); err != nil {
						t.Fatal(err)
					}
				} else if err := os.Symlink(original, filepath.Join(stage, "SKILL.md")); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(original)
			if err != nil {
				t.Fatal(err)
			}
			if err := Publish(context.Background(), c, "candidate", "candidate", "Test.", "1. Read source.\n2. Test."); err == nil {
				t.Fatal("unreadable catalog or unexpected staged resources published")
			}
			after, err := os.ReadFile(original)
			if err != nil || string(before) != string(after) {
				t.Fatal("existing entry changed", err)
			}
			if _, err := os.Stat(filepath.Join(c.Root, "candidate")); !os.IsNotExist(err) {
				t.Fatal("rejected publication became active", err)
			}
			if mode != "invalid-catalog" {
				if _, err := Definitions(c); err != nil {
					t.Fatal("failed publication disabled startup", err)
				}
			}
		})
	}
}
