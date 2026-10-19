package macosvm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/z-chenhao/erii/internal/computer"
	"golang.org/x/sys/unix"
)

// Config refers only to an explicitly provisioned, private Lume store. Endpoint
// and SSH material remain Host configuration; none enters a restoration manifest.
type Config struct {
	Root          string                       `yaml:"root" json:"root"`
	Endpoint      string                       `yaml:"endpoint" json:"endpoint"`
	Compatibility computer.Compatibility       `yaml:"compatibility" json:"compatibility"`
	Guest         SSHConfig                    `yaml:"guest" json:"guest"`
	Manifest      computer.RestorationManifest `yaml:"manifest" json:"manifest"`
}

func (c Config) Validate() error {
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() || u.Port() == "" || !filepath.IsAbs(c.Root) || filepath.Clean(c.Root) != c.Root {
		return computer.ErrInvalid
	}
	s := c.Compatibility
	if s.ProviderVersion != "0.6.1" || s.Architecture != "arm64" || s.GuestBuild == "" || s.DriverVersion == "" || s.DriverTeam == "" || s.DriverBundle != "com.trycua.driver" || len(s.DriverSHA256) != 64 {
		return computer.ErrCompatibility
	}
	if c.Manifest.Validate() != nil {
		return computer.ErrInvalid
	}
	return c.Guest.Validate()
}

type Guest interface {
	Ready(context.Context, string, computer.Compatibility) error
	Reopen(context.Context, string, computer.RestorationManifest) error
	Shutdown(context.Context, string) error
}

type Provider struct {
	config   Config
	client   *http.Client
	guest    Guest
	verified map[string]verifiedUnit
	// Boundary is deterministic crash injection. Production leaves it nil.
	Boundary func(string)
}

func New(c Config, guest Guest) (*Provider, error) {
	if c.Validate() != nil || guest == nil || privateRoot(c.Root) != nil {
		return nil, computer.ErrInvalid
	}
	var marker struct {
		Version int `json:"version"`
	}
	if readJSON(filepath.Join(c.Root, "erii-computer.json"), &marker) != nil || marker.Version != 1 {
		return nil, computer.ErrInvalid
	}
	for _, dir := range []string{"vms", "checkpoints", "operations", "base"} {
		if privateRoot(filepath.Join(c.Root, dir)) != nil {
			return nil, computer.ErrInvalid
		}
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 30 * time.Second, MaxIdleConns: 2}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Provider{config: c, client: client, guest: guest, verified: map[string]verifiedUnit{}}, nil
}

func (p *Provider) boundary(point string) {
	if p.Boundary != nil {
		p.Boundary(point)
	}
}

func (p *Provider) Close() { p.client.CloseIdleConnections() }

func (p *Provider) api(ctx context.Context, method, path string, input any) ([]byte, int, error) {
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return nil, 0, computer.ErrInvalid
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.config.Endpoint+"/lume"+path, body)
	if err != nil {
		return nil, 0, computer.ErrInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(req)
	if err != nil {
		return nil, 0, computer.ErrUnavailable
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024+1))
	if err != nil || len(raw) > 1024*1024 {
		return nil, response.StatusCode, computer.ErrUnavailable
	}
	return raw, response.StatusCode, nil
}

func (p *Provider) lock(ctx context.Context, r computer.Request) (*os.File, error) {
	if !identifier.MatchString(r.Instance.ComputerID) || !identifier.MatchString(r.Instance.NativeName) || !identifier.MatchString(r.OperationID) || r.Instance.Epoch < 1 || r.Fence == nil || !reflect.DeepEqual(r.Compatibility, p.config.Compatibility) || r.Manifest.Validate() != nil {
		return nil, computer.ErrInvalid
	}
	if err := r.Fence(ctx); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(p.config.Root, "provider.lock"), os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, computer.ErrUnavailable
	}
	if unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
		f.Close()
		return nil, computer.ErrBusy
	}
	// No other lifecycle operation can own a staging directory while this
	// cross-process lock is held. Published UUID units are never collected here.
	for _, subdir := range []string{"vms", "checkpoints"} {
		parent := filepath.Join(p.config.Root, subdir)
		if privateRoot(parent) != nil {
			f.Close()
			return nil, computer.ErrIntegrity
		}
		entries, err := os.ReadDir(parent)
		if err != nil {
			f.Close()
			return nil, computer.ErrIntegrity
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".stage-") {
				continue
			}
			stage := filepath.Join(parent, entry.Name())
			if !identifier.MatchString(strings.TrimPrefix(entry.Name(), ".stage-")) || privateRoot(stage) != nil {
				f.Close()
				return nil, computer.ErrIntegrity
			}
			if err := os.RemoveAll(stage); err != nil {
				f.Close()
				return nil, computer.ErrIntegrity
			}
		}
	}
	// A dedicated store is mandatory. Never enumerate or modify the user's
	// default VM registry, even when a name happens to match.
	raw, status, err := p.api(ctx, "GET", "/config", nil)
	var config struct {
		TelemetryEnabled *bool `json:"telemetryEnabled"`
		CachingEnabled   *bool `json:"cachingEnabled"`
		VMLocations      []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"vmLocations"`
	}
	if err != nil || status != 200 || json.Unmarshal(raw, &config) != nil || config.TelemetryEnabled == nil || *config.TelemetryEnabled || config.CachingEnabled == nil || *config.CachingEnabled || len(config.VMLocations) != 1 || config.VMLocations[0].Path != filepath.Join(p.config.Root, "vms") {
		f.Close()
		return nil, computer.ErrUnavailable
	}
	raw, status, err = p.api(ctx, "GET", "/host/status", nil)
	var host struct {
		Version string `json:"version"`
	}
	if err != nil || status != 200 || json.Unmarshal(raw, &host) != nil || host.Version != p.config.Compatibility.ProviderVersion {
		f.Close()
		return nil, computer.ErrCompatibility
	}
	return f, nil
}

func (p *Provider) state(ctx context.Context, name string) (status, ip string, err error) {
	raw, code, err := p.api(ctx, "GET", "/vms/"+name, nil)
	var value struct {
		Status string `json:"status"`
		IP     string `json:"ipAddress"`
	}
	if err != nil || code != 200 || json.Unmarshal(raw, &value) != nil || value.Status != "running" && value.Status != "stopped" {
		return "", "", computer.ErrUnavailable
	}
	if value.Status == "running" {
		ip := net.ParseIP(value.IP)
		if ip == nil || !ip.IsPrivate() && !ip.IsLoopback() {
			return "", "", computer.ErrUnavailable
		}
	}
	return value.Status, value.IP, nil
}

func (p *Provider) checkpoint(r computer.Request, cp computer.Checkpoint) (receipt, string, error) {
	if !identifier.MatchString(cp.ID) || cp.ComputerID != r.Instance.ComputerID || cp.SourceEpoch >= r.Instance.Epoch {
		return receipt{}, "", computer.ErrInvalid
	}
	dir := filepath.Join(p.config.Root, "checkpoints", cp.ID)
	var saved receipt
	if err := readJSON(filepath.Join(dir, "receipt.json"), &saved); err != nil {
		return saved, "", err
	}
	if saved.Checkpoint != cp || !reflect.DeepEqual(saved.Compatibility, r.Compatibility) {
		return saved, "", computer.ErrCompatibility
	}
	return saved, dir, nil
}

func (p *Provider) Ensure(ctx context.Context, r computer.Request) (computer.Instance, error) {
	lock, err := p.lock(ctx, r)
	if err != nil {
		return computer.Instance{}, err
	}
	defer lock.Close()
	return p.ensure(ctx, r, nil)
}

func (p *Provider) Restore(ctx context.Context, r computer.Request, cp computer.Checkpoint) (computer.Instance, error) {
	lock, err := p.lock(ctx, r)
	if err != nil {
		return computer.Instance{}, err
	}
	defer lock.Close()
	saved, _, err := p.checkpoint(r, cp)
	if err != nil {
		return computer.Instance{}, err
	}
	return p.ensure(ctx, r, &saved)
}

func (p *Provider) ensure(ctx context.Context, r computer.Request, saved *receipt) (computer.Instance, error) {
	dir := filepath.Join(p.config.Root, "vms", r.Instance.NativeName)
	operation := filepath.Join(p.config.Root, "operations", r.OperationID+".json")
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		source := filepath.Join(p.config.Root, "base")
		var base receipt
		if saved != nil {
			base = *saved
			source = filepath.Join(p.config.Root, "checkpoints", base.Checkpoint.ID)
		} else if readJSON(filepath.Join(source, "receipt.json"), &base) != nil || !reflect.DeepEqual(base.Compatibility, r.Compatibility) {
			return computer.Instance{}, computer.ErrCompatibility
		}
		if err := p.verify(ctx, source, base); err != nil {
			return computer.Instance{}, err
		}
		stage := filepath.Join(p.config.Root, "vms", ".stage-"+r.OperationID)
		// An unpublished stage contains only our three copied files. Rebuilding
		// it has no external side effect and cannot overwrite a committed unit.
		if err := os.RemoveAll(stage); err != nil {
			return computer.Instance{}, err
		}
		before, err := unitStamps(source)
		if err != nil {
			return computer.Instance{}, err
		}
		hashes, err := copyUnit(ctx, source, stage, base.Files)
		if err != nil {
			return computer.Instance{}, err
		}
		if !reflect.DeepEqual(hashes, base.Files) {
			return computer.Instance{}, computer.ErrIntegrity
		}
		after, err := unitStamps(source)
		if err != nil || !sameStamps(before, after) {
			return computer.Instance{}, computer.ErrIntegrity
		}
		if err := writeJSON(filepath.Join(stage, "erii-instance.json"), r.Instance); err != nil {
			return computer.Instance{}, err
		}
		p.boundary("before_instance_publish")
		if err := r.Fence(ctx); err != nil {
			return computer.Instance{}, err
		}
		if err := os.Rename(stage, dir); err != nil {
			return computer.Instance{}, err
		}
		if err := syncDir(filepath.Dir(dir)); err != nil {
			return computer.Instance{}, err
		}
	} else if err != nil {
		return computer.Instance{}, computer.ErrIntegrity
	}
	var owner computer.Instance
	if readJSON(filepath.Join(dir, "erii-instance.json"), &owner) != nil || owner != r.Instance {
		return computer.Instance{}, computer.ErrStale
	}
	status, ip, err := p.state(ctx, r.Instance.NativeName)
	if err != nil {
		return computer.Instance{}, err
	}
	if status == "stopped" {
		if _, err := os.Lstat(operation); !os.IsNotExist(err) {
			// The preceding process may have sent run. No Lume operation query
			// can prove a stopped request will not later start: fail closed.
			return computer.Instance{}, computer.ErrUnknown
		}
		if err := r.Fence(ctx); err != nil {
			return computer.Instance{}, err
		}
		if err := writeJSON(operation, r.Instance); err != nil {
			return computer.Instance{}, err
		}
		if err := syncDir(filepath.Dir(operation)); err != nil {
			return computer.Instance{}, err
		}
		p.boundary("before_run_request")
		_, code, err := p.api(ctx, "POST", "/vms/"+r.Instance.NativeName+"/run", map[string]any{"noDisplay": true, "vnc": "disabled"})
		if err != nil || code != 200 && code != 202 {
			status, ip, err = p.state(ctx, r.Instance.NativeName)
			if err != nil || status != "running" {
				return computer.Instance{}, computer.ErrUnknown
			}
		}
	}
	for {
		status, ip, err = p.state(ctx, r.Instance.NativeName)
		if err == nil && status == "running" && ip != "" {
			break
		}
		if err := pause(ctx); err != nil {
			return computer.Instance{}, err
		}
	}
	if err := p.guest.Ready(ctx, ip, r.Compatibility); err != nil {
		return computer.Instance{}, err
	}
	if saved != nil {
		if err := r.Fence(ctx); err != nil {
			return computer.Instance{}, err
		}
		if err := p.guest.Reopen(ctx, ip, saved.Manifest); err != nil {
			return computer.Instance{}, err
		}
	}
	return r.Instance, r.Fence(ctx)
}

func pause(ctx context.Context) error {
	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (p *Provider) Checkpoint(ctx context.Context, r computer.Request) (computer.Checkpoint, error) {
	lock, err := p.lock(ctx, r)
	if err != nil {
		return computer.Checkpoint{}, err
	}
	defer lock.Close()
	if !identifier.MatchString(r.CheckpointID) {
		return computer.Checkpoint{}, computer.ErrInvalid
	}
	dir := filepath.Join(p.config.Root, "vms", r.Instance.NativeName)
	target := filepath.Join(p.config.Root, "checkpoints", r.CheckpointID)
	hash, err := computer.Fingerprint(r.Manifest)
	if err != nil {
		return computer.Checkpoint{}, err
	}
	cp := computer.Checkpoint{ID: r.CheckpointID, ComputerID: r.Instance.ComputerID, SourceEpoch: r.Instance.Epoch, ManifestSHA256: hash}
	var existing receipt
	if readJSON(filepath.Join(target, "receipt.json"), &existing) == nil {
		if existing.Checkpoint != cp || !reflect.DeepEqual(existing.Compatibility, r.Compatibility) {
			return computer.Checkpoint{}, computer.ErrIntegrity
		}
		if err := p.verify(ctx, target, existing); err != nil {
			return computer.Checkpoint{}, err
		}
		return cp, r.Fence(ctx)
	}
	var instance computer.Instance
	if readJSON(filepath.Join(dir, "erii-instance.json"), &instance) != nil || instance != r.Instance {
		return computer.Checkpoint{}, computer.ErrStale
	}
	status, ip, err := p.state(ctx, r.Instance.NativeName)
	if err != nil {
		return computer.Checkpoint{}, err
	}
	if status == "running" {
		// A completed shutdown may leave the API cache running. Prove native
		// resource release first, before making any potentially destructive call.
		probe, stopped := quiescent(dir, true)
		if stopped == nil {
			probe.Close()
		} else {
			shutdownMarker := filepath.Join(p.config.Root, "operations", r.OperationID+"-shutdown.json")
			if _, e := os.Lstat(shutdownMarker); !os.IsNotExist(e) {
				return computer.Checkpoint{}, computer.ErrUnknown
			}
			if err := r.Fence(ctx); err != nil {
				return computer.Checkpoint{}, err
			}
			if err := writeJSON(shutdownMarker, r.Instance); err != nil {
				return computer.Checkpoint{}, err
			}
			if err := syncDir(filepath.Dir(shutdownMarker)); err != nil {
				return computer.Checkpoint{}, err
			}
			if err := p.guest.Shutdown(ctx, ip); err != nil {
				return computer.Checkpoint{}, computer.ErrUnknown
			}
		}
	}
	var native *os.File
	for {
		native, err = quiescent(dir, true)
		if err == nil {
			break
		}
		if !errors.Is(err, computer.ErrBusy) {
			return computer.Checkpoint{}, err
		}
		if err := pause(ctx); err != nil {
			return computer.Checkpoint{}, err
		}
	}
	if status == "running" {
		// Never hold an observer config lock across Lume /stop. Its native
		// cross-process fallback may signal any observed lock holder and may
		// replace config.json while clearing stale locks.
		native.Close()
		if err := r.Fence(ctx); err != nil {
			return computer.Checkpoint{}, err
		}
		raw, code, err := p.api(ctx, "POST", "/vms/"+r.Instance.NativeName+"/stop", map[string]bool{"force": false})
		var apiError struct {
			Message string `json:"message"`
		}
		if err != nil || code != 200 && (code != 400 || json.Unmarshal(raw, &apiError) != nil || apiError.Message != "Virtual machine not running: "+r.Instance.NativeName) {
			return computer.Checkpoint{}, computer.ErrUnknown
		}
		status, _, err = p.state(ctx, r.Instance.NativeName)
		if err != nil || status != "stopped" {
			return computer.Checkpoint{}, computer.ErrUnknown
		}
		native, err = quiescent(dir, true)
		if err != nil {
			return computer.Checkpoint{}, err
		}
	}
	defer native.Close()
	stage := filepath.Join(p.config.Root, "checkpoints", ".stage-"+r.OperationID)
	if err := os.RemoveAll(stage); err != nil {
		return computer.Checkpoint{}, err
	}
	hashes, err := copyUnit(ctx, dir, stage, nil)
	if err != nil {
		return computer.Checkpoint{}, err
	}
	saved := receipt{cp, r.Compatibility, r.Manifest, hashes}
	if err := writeJSON(filepath.Join(stage, "receipt.json"), saved); err != nil {
		return computer.Checkpoint{}, err
	}
	if err := syncDir(stage); err != nil {
		return computer.Checkpoint{}, err
	}
	p.boundary("before_checkpoint_publish")
	lockedInfo, lockErr := native.Stat()
	currentInfo, currentErr := os.Lstat(filepath.Join(dir, "config.json"))
	if lockErr != nil || currentErr != nil || !os.SameFile(lockedInfo, currentInfo) {
		return computer.Checkpoint{}, computer.ErrIntegrity
	}
	if err := r.Fence(ctx); err != nil {
		return computer.Checkpoint{}, err
	}
	if err := os.Rename(stage, target); err != nil {
		return computer.Checkpoint{}, err
	}
	if err := syncDir(filepath.Dir(target)); err != nil {
		return computer.Checkpoint{}, err
	}
	p.boundary("after_checkpoint_publish")
	if err := p.remember(target, saved); err != nil {
		return computer.Checkpoint{}, err
	}
	return cp, r.Fence(ctx)
}

func (p *Provider) Destroy(ctx context.Context, r computer.Request) error {
	lock, err := p.lock(ctx, r)
	if err != nil {
		return err
	}
	defer lock.Close()
	// Disposal can never remove the sole durable copy.
	if !identifier.MatchString(r.CheckpointID) {
		return computer.ErrInvalid
	}
	var saved receipt
	checkpointDir := filepath.Join(p.config.Root, "checkpoints", r.CheckpointID)
	if readJSON(filepath.Join(checkpointDir, "receipt.json"), &saved) != nil || saved.Checkpoint.ComputerID != r.Instance.ComputerID || saved.Checkpoint.SourceEpoch != r.Instance.Epoch || p.verify(ctx, checkpointDir, saved) != nil {
		return computer.ErrIntegrity
	}
	dir := filepath.Join(p.config.Root, "vms", r.Instance.NativeName)
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		return r.Fence(ctx)
	}
	var owner computer.Instance
	if readJSON(filepath.Join(dir, "erii-instance.json"), &owner) != nil || owner != r.Instance {
		return computer.ErrStale
	}
	native, err := quiescent(dir, false)
	if err != nil {
		return err
	}
	native.Close()
	if err := r.Fence(ctx); err != nil {
		return err
	}
	_, status, err := p.api(ctx, "DELETE", "/vms/"+r.Instance.NativeName, nil)
	if err != nil || status != 200 {
		if _, e := os.Lstat(dir); !os.IsNotExist(e) {
			return computer.ErrUnknown
		}
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		return computer.ErrUnknown
	}
	return r.Fence(ctx)
}

// ProvisionBase seals an already consented, stopped, dedicated clean image.
// It does not run Lume or inspect any default store. The caller is responsible
// for supplying a base with no personal files or credentials.
func ProvisionBase(ctx context.Context, root, source string, compatibility computer.Compatibility) error {
	if privateRoot(root) != nil || privateRoot(source) != nil || strings.HasPrefix(root+"/", source+"/") || strings.HasPrefix(source+"/", root+"/") {
		return computer.ErrInvalid
	}
	lock, err := quiescent(source, false)
	if err != nil {
		return err
	}
	defer lock.Close()
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		return computer.ErrInvalid
	}
	for _, dir := range []string{"vms", "checkpoints", "operations"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			return err
		}
	}
	hashes, err := copyUnit(ctx, source, filepath.Join(root, "base"), nil)
	if err != nil {
		return err
	}
	manifest := computer.RestorationManifest{Version: 1}
	hash, err := computer.Fingerprint(manifest)
	if err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(root, "base", "receipt.json"), receipt{Checkpoint: computer.Checkpoint{ID: "base", ManifestSHA256: hash}, Compatibility: compatibility, Manifest: manifest, Files: hashes}); err != nil {
		return err
	}
	if err := syncDir(filepath.Join(root, "base")); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(root, "erii-computer.json"), map[string]int{"version": 1}); err != nil {
		return err
	}
	return syncDir(root)
}
