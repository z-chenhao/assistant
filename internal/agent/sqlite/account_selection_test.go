package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/z-chenhao/assistant/internal/llm"
)

func TestAccountCatalogSelectionNeedsNoProbeAndFencesChanges(t *testing.T) {
	for _, mode := range []string{"success", "missing-model", "auth-change", "config-change", "event-failure"} {
		t.Run(mode, func(t *testing.T) {
			s, m, _, _, a := accountManager(t)
			ctx := context.Background()
			input := modelInput()
			input.ProviderKind, input.WireAPI, input.Endpoint = "openai_account", "openai_responses", ""
			input.AuthConnectionID = a.ConnectionID
			p, err := s.Models().AddProfile(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			c := control(s)
			c.AccountAuth = m.Check
			c.Probe = probeFixture{err: errors.New("account selection must not generate a probe")}
			c.AccountCatalog = func(context.Context, string) ([]llm.CatalogModel, error) {
				switch mode {
				case "missing-model":
					return nil, nil
				case "auth-change":
					if err := s.Models().UpdateAuthState(ctx, a.ConnectionID, a.Version, "logged_out", nil, nil); err != nil {
						t.Fatal(err)
					}
				case "config-change":
					if _, err := s.db.Exec(`UPDATE agent_config SET config_version=config_version+1`); err != nil {
						t.Fatal(err)
					}
				case "event-failure":
					if _, err := s.db.Exec(`CREATE TRIGGER reject_selection_event BEFORE INSERT ON event_outbox BEGIN SELECT RAISE(ABORT,'fault'); END`); err != nil {
						t.Fatal(err)
					}
				}
				return []llm.CatalogModel{{ModelID: p.ModelID, ContextWindow: 272000, MaxOutputTokens: 128000, ImageInput: true}}, nil
			}
			err = c.SelectWithProbe(ctx, p.ProfileID, p.Version, 1)
			stored, readErr := s.Models().Profile(ctx, p.ProfileID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if countRows(t, s, "model_probe") != 0 {
				t.Fatal("catalog selection fabricated behavioral evidence")
			}
			if mode != "success" {
				if err == nil || stored.Status != "unverified" || stored.Version != p.Version || stored.MaxOutputTokens != p.MaxOutputTokens {
					t.Fatal("failed selection changed the candidate", stored, err)
				}
				return
			}
			if err != nil || stored.Status != "ready" || stored.Version != p.Version+1 || stored.MaxOutputTokens != 128000 || !stored.Capabilities.ImageInput || !c.Available(ctx, p.ProfileID) {
				t.Fatal("catalog model is not ready", stored, err)
			}
			if _, err := m.Logout(ctx, a.ConnectionID); err != nil {
				t.Fatal(err)
			}
			if c.Available(ctx, p.ProfileID) {
				t.Fatal("logout left account model available")
			}
		})
	}
}

func TestFailedAccountCheckRevokesCatalogReadinessWithoutProbe(t *testing.T) {
	s, m, _, _, auth := accountManager(t)
	ctx := context.Background()
	input := modelInput()
	input.ProviderKind, input.WireAPI, input.Endpoint = "openai_account", "openai_responses", ""
	input.AuthConnectionID = auth.ConnectionID
	profile, err := s.Models().AddProfile(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	c := control(s)
	c.AccountAuth = m.Check
	c.AccountCatalog = func(context.Context, string) ([]llm.CatalogModel, error) {
		return []llm.CatalogModel{{ModelID: profile.ModelID, ContextWindow: 32768, MaxOutputTokens: 4096}}, nil
	}
	if err := c.Select(ctx, profile.ProfileID, profile.Version, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Models().InvalidateProbe(ctx, profile.ProfileID, profile.Version, "invalid"); err != nil {
		t.Fatal(err)
	}
	if !c.Available(ctx, profile.ProfileID) {
		t.Fatal("stale check revoked a newer selection")
	}
	c.Probe = probeFixture{err: llm.ModelNotFound}
	if _, err := c.Test(ctx, profile.ProfileID); err == nil {
		t.Fatal("failed account check succeeded")
	}
	if c.Available(ctx, profile.ProfileID) || countRows(t, s, "model_probe") != 0 {
		t.Fatal("failed check retained catalog readiness")
	}
	var status string
	if err := s.db.QueryRow(`SELECT setup_status FROM agent_config WHERE singleton=1`).Scan(&status); err != nil || status != "degraded" {
		t.Fatal("failed check did not close ingress", status, err)
	}
}
