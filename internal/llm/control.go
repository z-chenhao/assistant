package llm

import (
	"context"
	"errors"
	"time"
)

type ProbeRecord struct {
	ProfileVersion int64       `json:"profile_version"`
	AuthVersion    int64       `json:"auth_version"`
	BridgeIdentity string      `json:"bridge_identity"`
	Result         ProbeResult `json:"result"`
}

type ProfileStore interface {
	Profile(context.Context, string) (Profile, error)
	Auth(context.Context, string) (AuthConnection, error)
	RecordProbe(context.Context, Profile, ProbeRecord) error
	InvalidateProbe(context.Context, string, int64, string) error
	Probe(context.Context, string) (ProbeRecord, error)
	Select(context.Context, string, int64, int64, string) error
}

// Control owns user-driven Test/Select. No model Tool exposes this boundary.
type Control struct {
	Store          ProfileStore
	Probe          ModelProbe
	Secrets        SecretStore
	BridgeIdentity string
	BridgeError    error
}

func (c Control) CheckAuth(ctx context.Context, p Profile) (int64, error) {
	if p.AuthConnectionID == "" {
		return 0, nil
	}
	a, err := c.Store.Auth(ctx, p.AuthConnectionID)
	if err != nil || a.Status != "connected" || a.ExpiresAt != nil && !a.ExpiresAt.After(time.Now()) {
		return 0, AuthRequired
	}
	if c.Secrets == nil {
		return 0, AuthRequired
	}
	secret, err := c.Secrets.Get(ctx, a.CredentialRef)
	if err != nil || len(secret) == 0 {
		return 0, AuthRequired
	}
	for i := range secret {
		secret[i] = 0
	}
	return a.Version, nil
}

func (c Control) Test(ctx context.Context, id string) (ProbeRecord, error) {
	if c.BridgeError != nil {
		return ProbeRecord{}, c.BridgeError
	}
	if c.Probe == nil || c.BridgeIdentity == "" {
		return ProbeRecord{}, ErrNoBridge
	}
	p, err := c.Store.Profile(ctx, id)
	if err != nil {
		return ProbeRecord{}, err
	}
	if p.ReasoningEffort != "" {
		return ProbeRecord{}, ModelIncompatible
	}
	if err := p.ValidateActivation(); err != nil {
		return ProbeRecord{}, err
	}
	version, err := c.CheckAuth(ctx, p)
	if err != nil {
		return ProbeRecord{}, err
	}
	result, err := c.Probe.Run(ctx, p)
	if err != nil {
		return ProbeRecord{}, c.failedProbe(ctx, p, err)
	}
	if err := result.Validate(); err != nil {
		return ProbeRecord{}, c.failedProbe(ctx, p, err)
	}
	r := ProbeRecord{ProfileVersion: p.Version, AuthVersion: version, BridgeIdentity: c.BridgeIdentity, Result: result}
	return r, c.Store.RecordProbe(ctx, p, r)
}

func (c Control) failedProbe(ctx context.Context, p Profile, cause error) error {
	if ctx.Err() != nil || errors.Is(cause, GenerationCancelled) {
		return cause
	}
	status := "unavailable"
	if errors.Is(cause, AuthRequired) {
		status = "reauth_required"
	}
	if errors.Is(cause, ModelIncompatible) || errors.Is(cause, InvalidProviderResponse) {
		status = "invalid"
	}
	return errors.Join(cause, c.Store.InvalidateProbe(ctx, p.ProfileID, p.Version, status))
}

func (c Control) Select(ctx context.Context, id string, profileVersion, configVersion int64) error {
	if c.BridgeError != nil {
		return c.BridgeError
	}
	if c.Probe == nil || c.BridgeIdentity == "" {
		return ErrNoBridge
	}
	p, err := c.Store.Profile(ctx, id)
	if err != nil {
		return err
	}
	if p.Version != profileVersion {
		return ErrConflict
	}
	if err := p.ValidateActivation(); err != nil {
		return err
	}
	version, err := c.CheckAuth(ctx, p)
	if err != nil {
		return err
	}
	r, err := c.Store.Probe(ctx, id)
	if err != nil {
		return ErrUnverified
	}
	if r.ProfileVersion != p.Version || r.AuthVersion != version || r.BridgeIdentity != c.BridgeIdentity || r.Result.Validate() != nil {
		return ErrUnverified
	}
	return c.Store.Select(ctx, id, profileVersion, configVersion, c.BridgeIdentity)
}

// Available validates frozen Probe provenance and credentials on every startup.
// A saved Fake result is unusable without the same explicit test injection.
func (c Control) Available(ctx context.Context, id string) bool {
	if c.Probe == nil || c.BridgeIdentity == "" {
		return false
	}
	p, err := c.Store.Profile(ctx, id)
	if err != nil || p.Status != "ready" || p.VerifiedAt == nil || p.ValidateActivation() != nil {
		return false
	}
	version, err := c.CheckAuth(ctx, p)
	if err != nil {
		return false
	}
	r, err := c.Store.Probe(ctx, id)
	return err == nil && r.ProfileVersion == p.Version && r.AuthVersion == version && r.BridgeIdentity == c.BridgeIdentity && r.Result.Validate() == nil
}
