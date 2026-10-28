package computer_test

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/computer"
	"github.com/z-chenhao/erii/internal/computer/fake"
	"github.com/z-chenhao/erii/internal/tool"
)

type store struct {
	state computer.UseState
	lost  bool
}

func (s *store) Load(context.Context) (computer.UseState, error)   { return s.state, nil }
func (s *store) Save(_ context.Context, v computer.UseState) error { s.state = v; return nil }
func (s *store) Fence(context.Context) error {
	if s.lost {
		return computer.ErrStale
	}
	return nil
}
func (s *store) OpenArtifact(_ context.Context, ref string) (io.ReadCloser, error) {
	if ref != "artifact:allowed" {
		return nil, computer.ErrIntegrity
	}
	return io.NopCloser(strings.NewReader("private data")), nil
}
func call(t *testing.T, r computer.UseRuntime, operation, token, method string, args ...any) tool.Result {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"method": method, "handle_token": token, "args": args})
	v, e := r.Call(context.Background(), "builtin__computer_"+operation, string(raw))
	if e != nil {
		t.Fatal(e)
	}
	if v.Output != nil {
		if _, err := io.Copy(io.Discard, v.Output.Reader); err != nil {
			t.Fatal(err)
		}
		if closer, ok := v.Output.Reader.(io.Closer); ok {
			closer.Close()
		}
		v.Text = v.Output.ResultJSON("artifact:screenshot")
		v.Output = nil
	}
	if receipt, ok := v.HostFence.(computer.ObservationReceipt); ok && v.Error == nil {
		state, _ := r.Store.Load(context.Background())
		h := state.Handles[receipt.Token]
		if receipt.UpdateAX {
			h.AXReady = true
		}
		if receipt.UpdateScreenshot {
			h.ScreenshotReady = true
		}
		if err := r.Store.Save(context.Background(), state); err != nil {
			t.Fatal(err)
		}
	}
	return v
}
func value(t *testing.T, r tool.Result) map[string]any {
	t.Helper()
	if r.Error != nil {
		t.Fatal(r.Error)
	}
	var v map[string]any
	if json.Unmarshal([]byte(r.Text), &v) != nil {
		t.Fatal(r.Text)
	}
	return v
}
func code(t *testing.T, v tool.Result, want string) {
	t.Helper()
	if v.Error == nil || v.Error.Code != want {
		t.Fatal(v, want)
	}
}
func TestUseRevisionsLeaseAndIndependentScreenshots(t *testing.T) {
	s := &store{}
	d := &fake.Driver{Geometry: "100x100"}
	r := computer.UseRuntime{Driver: d, Store: s, Instance: computer.Instance{ComputerID: "c", Epoch: 1}, Lease: "l"}
	token := value(t, call(t, r, "bind", "", "getApp", "notes"))["handle_token"].(string)
	code(t, call(t, r, "invoke", token, "click", []any{1, 2}), "SCREENSHOT_REQUIRED")
	ax := value(t, call(t, r, "invoke", token, "getAXState"))
	rev := ax["revision"]
	value(t, call(t, r, "invoke", token, "getScreenshot"))
	if s.state.Handles[token].AXRevision != rev {
		t.Fatal("screenshot changed AX revision")
	}
	value(t, call(t, r, "invoke", token, "performSecondaryAction", map[string]any{"observationRevision": rev, "index": 0}, "increment"))
	code(t, call(t, r, "invoke", token, "setValue", map[string]any{"observationRevision": rev, "index": 0}, "wrong"), "STALE_ELEMENT_REFERENCE")
	value(t, call(t, r, "invoke", token, "getAXState"))
	value(t, call(t, r, "invoke", token, "getAXState"))
	code(t, call(t, r, "invoke", token, "click", map[string]any{"observationRevision": rev, "index": 0}), "STALE_ELEMENT_REFERENCE")
	code(t, call(t, r, "invoke", token, "performSecondaryAction", 0, "delete"), "PERMISSION_DENIED")
	r.Lease = "other"
	code(t, call(t, r, "invoke", token, "getAXState"), "STALE_SURFACE_HANDLE")
	r.Lease = "l"
	r.Instance.Epoch++
	code(t, call(t, r, "invoke", token, "getAXState"), "STALE_SURFACE_HANDLE")
}
func TestUseInventoryCapabilitiesWaitAndTransfers(t *testing.T) {
	s := &store{}
	d := &fake.Driver{FailInventory: "browsers"}
	r := computer.UseRuntime{Driver: d, Store: s, Instance: computer.Instance{ComputerID: "c", Epoch: 1}, Lease: "l"}
	v := value(t, call(t, r, "discover", "", "getState"))
	if len(v["apps"].([]any)) != 1 || len(v["errors"].([]any)) != 1 {
		t.Fatal(v)
	}
	code(t, call(t, r, "bind", "", "getApp", "missing"), "NOT_FOUND")
	if len(d.Calls) != 4 {
		t.Fatal("getApp launched", d.Calls)
	}
	token := value(t, call(t, r, "bind", "", "getTab", "fixture-tab"))["handle_token"].(string)
	code(t, call(t, r, "invoke", token, "capabilities.get", "playwright"), "PERMISSION_DENIED")
	call(t, r, "invoke", token, "capabilities.list")
	cap := value(t, call(t, r, "invoke", token, "capabilities.get", "playwright"))["handle_token"].(string)
	call(t, r, "invoke", token, "goto", "https://example.test")
	code(t, call(t, r, "invoke", cap, "playwright.count", map[string]any{"selector": "input"}), "STALE_CAPABILITY_HANDLE")
	call(t, r, "invoke", token, "getAXState")
	timed := call(t, r, "invoke", token, "waitForChange", map[string]any{"timeoutMs": 1})
	code(t, timed, "WAIT_TIMEOUT")
	if timed.Error.Details["observation"] == nil {
		t.Fatal("no summary")
	}
	for _, p := range []string{"../a", "/etc/passwd", "imports/../a", "imports/a/b", "imports/..", "imports/a\\b", "https://a"} {
		code(t, call(t, r, "discover", "", "importArtifact", map[string]any{"artifactRef": "artifact:allowed", "destination": p}), "PERMISSION_DENIED")
	}
	imported := value(t, call(t, r, "discover", "", "importArtifact", map[string]any{"artifactRef": "artifact:allowed", "destination": "imports/test"}))
	if imported["path"] != "imports/test" {
		t.Fatal(imported)
	}
	code(t, call(t, r, "discover", "", "exportArtifact", map[string]any{"path": "downloads/unknown"}), "PERMISSION_DENIED")
	value(t, call(t, r, "discover", "", "exportArtifact", map[string]any{"path": "imports/test"}))
	code(t, call(t, r, "discover", "", "importArtifact", map[string]any{"artifactRef": "artifact:cross-task", "destination": "imports/cross"}), "NOT_FOUND")
}
func TestEveryMutationHasFrozenApprovalAndNeverReplay(t *testing.T) {
	p := tool.Spec{Permission: "read_only"}.Policy()
	for _, m := range []string{"click", "setValue", "goto", "playwright.upload", "playwright.download"} {
		v := computer.CallPolicy("builtin__computer_invoke", `{"method":"`+m+`","args":[]}`, p)
		if v.Risk != "high" || v.Replay != "never" {
			t.Fatal(m, v)
		}
	}
	for _, m := range []string{"importArtifact", "exportArtifact", "launchApp"} {
		v := computer.CallPolicy("builtin__computer_discover", `{"method":"`+m+`","args":[]}`, p)
		if v.Risk != "high" || v.Replay != "never" {
			t.Fatal(m, v)
		}
	}
}

type deadlineSensitiveStore struct{ store }

func (s *deadlineSensitiveStore) Save(ctx context.Context, v computer.UseState) error {
	if _, bounded := ctx.Deadline(); bounded {
		return computer.ErrUnavailable
	}
	return s.store.Save(ctx, v)
}
func TestWaitDeadlineDoesNotCancelStateCommit(t *testing.T) {
	s := &deadlineSensitiveStore{}
	r := computer.UseRuntime{Driver: &fake.Driver{}, Store: s, Instance: computer.Instance{ComputerID: "c", Epoch: 1}, Lease: "l"}
	token := value(t, call(t, r, "bind", "", "getApp", "notes"))["handle_token"].(string)
	call(t, r, "invoke", token, "getAXState")
	code(t, call(t, r, "invoke", token, "waitForChange", map[string]any{"timeoutMs": 1}), "WAIT_TIMEOUT")
}

func TestAXDiffRetainsBaseAndActualChanges(t *testing.T) {
	s := &store{}
	d := &fake.Driver{}
	r := computer.UseRuntime{Driver: d, Store: s, Instance: computer.Instance{ComputerID: "c", Epoch: 1}, Lease: "l"}
	token := value(t, call(t, r, "bind", "", "getApp", "notes"))["handle_token"].(string)
	first := value(t, call(t, r, "invoke", token, "getAXState"))
	d.Text = "changed"
	second := value(t, call(t, r, "invoke", token, "getAXState"))
	if second["kind"] != "diff" || second["baseRevision"] != first["revision"] || !strings.Contains(second["state"].(string), "@@ 0,1,1 @@\n- ") || !strings.Contains(second["state"].(string), "+ [0] Notes changed") {
		t.Fatal(first, second)
	}
	full := value(t, call(t, r, "invoke", token, "getAXState", map[string]any{"full": true}))
	if full["kind"] != "full" || strings.Contains(full["state"].(string), "@@") {
		t.Fatal(full)
	}
}

type changingDriver struct {
	fake.Driver
	reads int
}

func (d *changingDriver) Observe(ctx context.Context, i computer.Instance, target computer.Target, lease string, opts computer.ObserveOptions) (computer.Capture, error) {
	d.reads++
	if d.reads == 3 {
		d.Text = "changed after an unchanged poll"
	}
	return d.Driver.Observe(ctx, i, target, lease, opts)
}
func TestWaitDiffUsesLastDeliveredRevision(t *testing.T) {
	s := &store{}
	r := computer.UseRuntime{Driver: &changingDriver{}, Store: s, Instance: computer.Instance{ComputerID: "c", Epoch: 1}, Lease: "l"}
	token := value(t, call(t, r, "bind", "", "getApp", "notes"))["handle_token"].(string)
	first := value(t, call(t, r, "invoke", token, "getAXState"))
	changed := value(t, call(t, r, "invoke", token, "waitForChange", map[string]any{"timeoutMs": 1000, "sinceRevision": first["revision"]}))
	if changed["baseRevision"] != first["revision"] || !strings.Contains(changed["state"].(string), "changed after an unchanged poll") {
		t.Fatal(first, changed)
	}
}

// A transport deadline can fire before context's cancellation goroutine runs.
type transportDeadlineDriver struct{ fake.Driver }

func (d *transportDeadlineDriver) Observe(ctx context.Context, i computer.Instance, target computer.Target, lease string, opts computer.ObserveOptions) (computer.Capture, error) {
	if deadline, ok := ctx.Deadline(); ok {
		for time.Now().Before(deadline) {
		}
		return computer.Capture{}, computer.ErrUnavailable
	}
	return d.Driver.Observe(ctx, i, target, lease, opts)
}
func TestWaitTransportDeadlineStillReturnsObservationSummary(t *testing.T) {
	s := &store{}
	r := computer.UseRuntime{Driver: &transportDeadlineDriver{}, Store: s, Instance: computer.Instance{ComputerID: "c", Epoch: 1}, Lease: "l"}
	token := value(t, call(t, r, "bind", "", "getApp", "notes"))["handle_token"].(string)
	call(t, r, "invoke", token, "getAXState")
	result := call(t, r, "invoke", token, "waitForChange", map[string]any{"timeoutMs": 1})
	code(t, result, "WAIT_TIMEOUT")
	if result.Error.Details["observation"] == nil {
		t.Fatal(result)
	}
}
