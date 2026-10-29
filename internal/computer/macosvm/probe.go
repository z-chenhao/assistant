package macosvm

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"

	"github.com/z-chenhao/collie/internal/computer"
)

// Probe checks the configured control plane and sealed base without cloning,
// booting, stopping or changing a VM. Guest identity is still checked on Ensure.
func (p *Provider) Probe(ctx context.Context) error {
	if e := p.checkHost(ctx); e != nil {
		return e
	}
	var e error
	var base receipt
	dir := filepath.Join(p.config.Root, "base")
	if readJSON(filepath.Join(dir, "receipt.json"), &base) != nil || !reflect.DeepEqual(base.Compatibility, p.config.Compatibility) {
		return computer.ErrCompatibility
	}
	if e = p.verify(ctx, dir, base); e != nil {
		return e
	}
	guest, ok := p.guest.(SSHGuest)
	if !ok || guest.Secrets == nil {
		return computer.ErrUnavailable
	}
	key, e := guest.Secrets.Get(ctx, guest.Config.PasswordRef)
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()
	if e != nil || len(key) == 0 {
		return computer.ErrUnavailable
	}
	return nil
}

func (p *Provider) checkHost(ctx context.Context) error {
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
		return computer.ErrUnavailable
	}
	raw, status, err = p.api(ctx, "GET", "/host/status", nil)
	var host struct {
		Version string `json:"version"`
	}
	if err != nil || status != 200 || json.Unmarshal(raw, &host) != nil || host.Version != p.config.Compatibility.ProviderVersion {
		return computer.ErrCompatibility
	}
	return nil
}
