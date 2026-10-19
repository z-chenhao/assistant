package macosvm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/computer"
	"github.com/z-chenhao/erii/internal/tool"
	"golang.org/x/sys/unix"
)

// lumeFixture models the pinned API's native lock and stale SharedVM cache,
// including an ambiguous run response. It never starts a macOS VM or account.
type lumeFixture struct {
	mu                 sync.Mutex
	root               string
	locks              map[string]*os.File
	running            map[string]bool
	runs, shuts, stops int
	ambiguous, hang    bool
	manifest           computer.RestorationManifest
	server             *httptest.Server
}

func mustFile(t *testing.T, name, text string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func testCompatibility() computer.Compatibility {
	return computer.Compatibility{ProviderVersion: "0.6.1", GuestBuild: "25F84", DriverVersion: "0.36.0", DriverSHA256: strings.Repeat("a", 64), DriverTeam: "YCK386LBJ7", DriverBundle: "com.trycua.driver", Architecture: "arm64"}
}

func lifecycleFixture(t *testing.T) (*Provider, *lumeFixture, computer.Request) {
	t.Helper()
	root, source := t.TempDir(), t.TempDir()
	var err error
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{root, source} {
		if err := os.Chmod(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range unitFiles {
		mustFile(t, filepath.Join(source, name), "base-"+name)
	}
	if err := ProvisionBase(context.Background(), root, source, testCompatibility()); err != nil {
		t.Fatal("base provisioning", err)
	}
	f := &lumeFixture{root: root, locks: map[string]*os.File{}, running: map[string]bool{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(func() {
		f.server.Close()
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, lock := range f.locks {
			lock.Close()
		}
	})
	cfg := Config{Root: root, Endpoint: f.server.URL, Compatibility: testCompatibility(), Manifest: computer.RestorationManifest{Version: 1}, Guest: SSHConfig{User: "lume", Port: 22, PasswordRef: "test:guest", HostKeySHA256: "SHA256:" + strings.Repeat("a", 43)}}
	p, err := New(cfg, f)
	if err != nil {
		t.Fatal(err)
	}
	r := computer.Request{Instance: computer.Instance{ComputerID: "logical-computer", Epoch: 1, NativeName: "instance-1"}, OperationID: "ensure-1", CheckpointID: "checkpoint-1", Compatibility: cfg.Compatibility, Manifest: computer.RestorationManifest{Version: 1, Applications: []computer.ApplicationIntent{{BundleID: "com.apple.TextEdit", Documents: []string{"/Users/lume/work.txt"}}}, ArtifactRefs: []string{"artifact:stable-fixture"}}, Fence: func(context.Context) error { return nil }}
	return p, f, r
}

func (f *lumeFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/lume/config" {
		json.NewEncoder(w).Encode(map[string]any{"telemetryEnabled": false, "cachingEnabled": false, "vmLocations": []any{map[string]string{"name": "owned", "path": filepath.Join(f.root, "vms")}}})
		return
	}
	if r.URL.Path == "/lume/host/status" {
		json.NewEncoder(w).Encode(map[string]string{"version": "0.6.1"})
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/lume/vms/"), "/")
	if len(parts) < 1 || !identifier.MatchString(parts[0]) {
		http.Error(w, "invalid", 400)
		return
	}
	name := parts[0]
	dir := filepath.Join(f.root, "vms", name)
	if r.Method == "GET" {
		if _, err := os.Stat(dir); err != nil {
			http.Error(w, "missing", 400)
			return
		}
		status := "stopped"
		if f.running[name] {
			status = "running"
		}
		json.NewEncoder(w).Encode(map[string]string{"status": status, "ipAddress": "192.168.64.2"})
		return
	}
	if r.Method == "DELETE" {
		if f.locks[name] != nil {
			http.Error(w, "busy", 400)
			return
		}
		if os.RemoveAll(dir) != nil {
			http.Error(w, "failure", 400)
			return
		}
		delete(f.running, name)
		w.Write([]byte(`{}`))
		return
	}
	if len(parts) != 2 {
		http.Error(w, "invalid", 400)
		return
	}
	switch parts[1] {
	case "run":
		f.runs++
		var input map[string]any
		if json.NewDecoder(r.Body).Decode(&input) != nil || input["noDisplay"] != true || input["vnc"] != "disabled" || len(input) != 2 {
			http.Error(w, "unsafe run", 400)
			return
		}
		if f.ambiguous {
			http.Error(w, "transport lost", 503)
			return
		}
		lock, err := os.Open(filepath.Join(dir, "config.json"))
		if err != nil || unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
			http.Error(w, "lock", 400)
			return
		}
		f.locks[name] = lock
		f.running[name] = true
		if err := os.WriteFile(filepath.Join(dir, "sessions.json"), []byte(`{"secret":"must not migrate"}`), 0600); err != nil {
			http.Error(w, "marker", 400)
			return
		}
		w.Write([]byte(`{}`))
	case "stop":
		f.stops++
		if f.locks[name] != nil {
			http.Error(w, "native lock held", 409)
			return
		}
		probe, err := os.Open(filepath.Join(dir, "config.json"))
		if err != nil {
			http.Error(w, "probe", 500)
			return
		}
		if unix.Flock(int(probe.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
			probe.Close()
			http.Error(w, "observer would be signaled", 409)
			return
		}
		probe.Close()
		payload, err := os.ReadFile(filepath.Join(dir, "config.json"))
		if err != nil {
			http.Error(w, "read", 500)
			return
		}
		if os.WriteFile(filepath.Join(dir, "config.json.replacement"), payload, 0600) != nil || os.Rename(filepath.Join(dir, "config.json.replacement"), filepath.Join(dir, "config.json")) != nil {
			http.Error(w, "replace", 500)
			return
		}
		if _, err := os.Stat(filepath.Join(dir, "sessions.json")); !os.IsNotExist(err) {
			http.Error(w, "marker held", 409)
			return
		}
		f.running[name] = false
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"message": "Virtual machine not running: " + name})
	default:
		http.Error(w, "invalid", 400)
	}
}

func (f *lumeFixture) Ready(context.Context, string, computer.Compatibility) error { return nil }
func (f *lumeFixture) Reopen(_ context.Context, _ string, m computer.RestorationManifest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.manifest = m
	return nil
}
func (f *lumeFixture) Shutdown(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shuts++
	if f.hang {
		return nil
	}
	for name, lock := range f.locks {
		lock.Close()
		delete(f.locks, name)
		os.Remove(filepath.Join(f.root, "vms", name, "sessions.json"))
	}
	return nil
}

func TestLifecyclePreservesWholeUnitAndIntent(t *testing.T) {
	p, f, r := lifecycleFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := p.Ensure(ctx, r); err != nil {
		t.Fatal(err)
	}
	for _, name := range unitFiles {
		mustFile(t, filepath.Join(p.config.Root, "vms", r.Instance.NativeName, name), "edited-"+name)
	}
	r.OperationID = "idle-1"
	cp, err := p.Checkpoint(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Destroy(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p.config.Root, "vms", r.Instance.NativeName)); !os.IsNotExist(err) {
		t.Fatal("instance retained", err)
	}
	files, err := os.ReadDir(filepath.Join(p.config.Root, "checkpoints", cp.ID))
	if err != nil || len(files) != 4 {
		t.Fatal(files, err)
	}
	next := r
	next.Instance.Epoch = 2
	next.Instance.NativeName = "instance-2"
	next.OperationID = "restore-2"
	if instance, err := p.Restore(ctx, next, cp); err != nil || instance.ComputerID != r.Instance.ComputerID || instance.Epoch != 2 {
		t.Fatal(instance, err)
	}
	for _, name := range unitFiles {
		raw, err := os.ReadFile(filepath.Join(p.config.Root, "vms", next.Instance.NativeName, name))
		if err != nil || string(raw) != "edited-"+name {
			t.Fatal(name, string(raw), err)
		}
	}
	if !reflect.DeepEqual(f.manifest, r.Manifest) || f.shuts != 1 || f.stops != 1 || f.runs != 2 {
		t.Fatal(f.manifest, f.shuts, f.stops, f.runs)
	}
	oldCalls := f.runs
	next.Fence = func(context.Context) error { return computer.ErrStale }
	if _, err := p.Ensure(ctx, next); !errors.Is(err, computer.ErrStale) || f.runs != oldCalls {
		t.Fatal("old epoch acted", err)
	}
}

func TestCorruptCheckpointCannotStart(t *testing.T) {
	p, f, r := lifecycleFixture(t)
	ctx := context.Background()
	if _, err := p.Ensure(ctx, r); err != nil {
		t.Fatal(err)
	}
	r.OperationID = "idle-1"
	cp, err := p.Checkpoint(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	mustFile(t, filepath.Join(p.config.Root, "checkpoints", cp.ID, "nvram.bin"), "corruption")
	r.Instance.Epoch = 2
	r.Instance.NativeName = "instance-2"
	r.OperationID = "restore-2"
	if _, err := p.Restore(ctx, r, cp); !errors.Is(err, computer.ErrIntegrity) || f.runs != 1 {
		t.Fatal("corrupt unit started", err, f.runs)
	}
}

func TestUnknownRunIsNeverResent(t *testing.T) {
	p, f, r := lifecycleFixture(t)
	f.ambiguous = true
	for range 2 {
		if _, err := p.Ensure(context.Background(), r); !errors.Is(err, computer.ErrUnknown) {
			t.Fatal(err)
		}
	}
	if f.runs != 1 {
		t.Fatal("ambiguous start repeated", f.runs)
	}
}

func TestNoCheckpointOrPoweroffWhileNativeLockHeld(t *testing.T) {
	p, f, r := lifecycleFixture(t)
	f.hang = true
	if _, err := p.Ensure(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	r.OperationID = "idle-1"
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := p.Checkpoint(ctx, r); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if f.stops != 0 {
		t.Fatal("poweroff attempted", f.stops)
	}
	if _, err := os.Stat(filepath.Join(p.config.Root, "checkpoints", r.CheckpointID)); !os.IsNotExist(err) {
		t.Fatal("published live disk", err)
	}
	if _, err := p.Checkpoint(context.Background(), r); !errors.Is(err, computer.ErrUnknown) || f.shuts != 1 {
		t.Fatal("shutdown blindly repeated", err, f.shuts)
	}
}

func TestStoppedCrashMarkerReclaimedOnlyAfterNativeRelease(t *testing.T) {
	p, f, r := lifecycleFixture(t)
	if _, err := p.Ensure(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.locks[r.Instance.NativeName].Close()
	delete(f.locks, r.Instance.NativeName)
	f.mu.Unlock()
	r.OperationID = "recover-1"
	if _, err := p.Checkpoint(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if f.shuts != 0 || f.stops != 1 {
		t.Fatal("dead Guest shutdown requested", f.shuts, f.stops)
	}
}

func TestComputerBridgeDefinitions(t *testing.T) {
	if _, err := tool.NewRegistry(computer.Definitions("pinned-fixture")); err != nil {
		t.Fatal(err)
	}
}

func TestCheckpointProcessDeathHelper(t *testing.T) {
	root := os.Getenv("ERII_COMPUTER_CRASH_ROOT")
	if root == "" {
		return
	}
	var cfg Config
	if readJSON(filepath.Join(root, "fixture-config.json"), &cfg) != nil {
		os.Exit(81)
	}
	p, err := New(cfg, &lumeFixture{})
	if err != nil {
		os.Exit(82)
	}
	p.Boundary = func(point string) {
		if point == os.Getenv("ERII_COMPUTER_CRASH_POINT") {
			os.Exit(91)
		}
	}
	var r computer.Request
	if readJSON(filepath.Join(root, "fixture-request.json"), &r) != nil {
		os.Exit(83)
	}
	r.Fence = func(context.Context) error { return nil }
	if _, err = p.Checkpoint(context.Background(), r); err != nil {
		os.Exit(84)
	}
	os.Exit(0)
}

func TestCheckpointProcessDeathNeverPublishesMixedUnit(t *testing.T) {
	for _, point := range []string{"before_checkpoint_publish", "after_checkpoint_publish"} {
		t.Run(point, func(t *testing.T) {
			p, f, r := lifecycleFixture(t)
			ctx := context.Background()
			if _, err := p.Ensure(ctx, r); err != nil {
				t.Fatal(err)
			}
			if err := f.Shutdown(ctx, "192.168.64.2"); err != nil {
				t.Fatal(err)
			}
			r.OperationID = "idle-1"
			if err := writeJSON(filepath.Join(p.config.Root, "fixture-config.json"), p.config); err != nil {
				t.Fatal(err)
			}
			if err := writeJSON(filepath.Join(p.config.Root, "fixture-request.json"), r); err != nil {
				t.Fatal(err)
			}
			command := exec.Command(os.Args[0], "-test.run=^TestCheckpointProcessDeathHelper$")
			command.Env = append(os.Environ(), "ERII_COMPUTER_CRASH_ROOT="+p.config.Root, "ERII_COMPUTER_CRASH_POINT="+point)
			output, err := command.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 91 {
				t.Fatal(string(output), err)
			}
			_, statErr := os.Stat(filepath.Join(p.config.Root, "checkpoints", r.CheckpointID))
			if point == "before_checkpoint_publish" && !os.IsNotExist(statErr) || point == "after_checkpoint_publish" && statErr != nil {
				t.Fatal("wrong atomic boundary", statErr)
			}
			cp, err := p.Checkpoint(ctx, r)
			if err != nil {
				t.Fatal("recovery", err)
			}
			var saved receipt
			if err := readJSON(filepath.Join(p.config.Root, "checkpoints", cp.ID, "receipt.json"), &saved); err != nil {
				t.Fatal(err)
			}
			if err := verifyUnit(ctx, filepath.Join(p.config.Root, "checkpoints", cp.ID), saved); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUnitSymlinkAndManifestRejection(t *testing.T) {
	p, _, r := lifecycleFixture(t)
	if err := os.Remove(filepath.Join(p.config.Root, "base", "disk.img")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(p.config.Root, "base", "disk.img")); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Ensure(context.Background(), r); !errors.Is(err, computer.ErrIntegrity) {
		t.Fatal(err)
	}
	for _, path := range []string{"../personal", "/Users/lume/../secret", "/Users/lume/a\ncommand"} {
		m := computer.RestorationManifest{Version: 1, Applications: []computer.ApplicationIntent{{BundleID: "com.apple.TextEdit", Documents: []string{path}}}}
		if m.Validate() == nil {
			t.Fatal(fmt.Sprintf("invalid intent accepted: %q", path))
		}
	}
}
