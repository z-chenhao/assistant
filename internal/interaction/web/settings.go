package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/z-chenhao/assistant/internal/llm"
)

type settingsView struct {
	TargetUnavailable     bool
	Section               string
	Provider              string
	TargetModel           string
	DirectModels          []modelResource
	Account               *llm.AuthConnection
	Catalog               []llm.CatalogModel
	CatalogLoaded         bool
	CatalogError          bool
	AccountModels         map[string][]modelResource
	Target                string
	Model, Endpoint, Name string
	ImageInput            bool
}

type modelResource struct {
	llm.Profile
	Current, Verified, Open, CanTest bool
	ConfigVersion                    int64
}

func resourceModelQuery(id, notice string) url.Values {
	query := url.Values{"model": {id}}
	if notice != "" {
		query.Set("notice", notice)
	}
	return query
}

func resourceModelURL(id, notice string) string {
	return "/settings/setup?" + resourceModelQuery(id, notice).Encode() + "#model-" + url.PathEscape(id)
}

func (s *Server) loadSettings(ctx context.Context, r *http.Request, p *page) error {
	v := &settingsView{Section: "resources", Provider: r.URL.Query().Get("provider"), Endpoint: "http://127.0.0.1:8000/v1"}
	p.Settings = v
	if r.URL.Path == "/settings/preferences" {
		v.Section = "preferences"
		return nil
	}
	if r.URL.Path == "/settings/notifications" {
		v.Section = "notifications"
		var err error
		p.Bindings, err = s.Core.ChannelBindings(ctx)
		if err != nil {
			return err
		}
		p.Preference, err = s.Core.ChannelPreference(ctx)
		if err != nil {
			return err
		}
		v.Target = p.Preference.Channel + ":"
		if p.Preference.BindingID != nil {
			v.Target += *p.Preference.BindingID
		}
		if p.Preference.Channel == "" {
			v.Target = "auto:"
		}
		if v.Target != "auto:" && v.Target != "tui:" {
			v.TargetUnavailable = true
			for _, b := range p.Bindings {
				if b.Enabled && bindingTarget(b) == v.Target {
					v.TargetUnavailable = false
				}
			}
		}
		return nil
	}
	if r.URL.Path == "/settings/models/new" {
		v.Section = "new"
		if v.Provider == "" {
			v.Provider = "chatgpt"
		}
		if v.Provider != "compatible" && v.Provider != "chatgpt" {
			return llm.ErrConflict
		}
		if v.Provider == "compatible" {
			return nil
		}
	}
	if s.Accounts != nil {
		var err error
		p.Accounts, err = s.Accounts.Store.Accounts(ctx)
		if err != nil {
			return err
		}
	}
	if v.Section == "resources" {
		return s.loadResourceModels(ctx, r, p)
	}

	account := r.URL.Query().Get("account")
	if account == "" {
		account = p.AccountID
	}
	for i := range p.Accounts {
		if p.Accounts[i].ConnectionID == account {
			v.Account = &p.Accounts[i]
			break
		}
	}
	if account != "" && v.Account == nil {
		return errPageNotFound
	}
	if v.Account == nil && len(p.Accounts) == 1 {
		v.Account = &p.Accounts[0]
	}
	return nil
}

func (s *Server) loadResourceModels(ctx context.Context, r *http.Request, p *page) error {
	profiles, err := s.Models.ListProfiles(ctx)
	if err != nil {
		return err
	}
	v := p.Settings
	v.AccountModels = make(map[string][]modelResource, len(p.Accounts))
	connectedAccounts := make(map[string]bool, len(p.Accounts))
	for _, account := range p.Accounts {
		v.AccountModels[account.ConnectionID] = nil
		connectedAccounts[account.ConnectionID] = account.Status == "connected"
	}
	requested := r.URL.Query().Get("model")
	notice := r.URL.Query().Get("notice")
	if strings.HasPrefix(r.URL.Path, "/settings/models/") {
		requested = strings.TrimPrefix(r.URL.Path, "/settings/models/")
	}
	for _, profile := range profiles {
		current := profile.ProfileID == activeProfile(p.Status)
		verified := s.profileVerified(ctx, profile)
		row := modelResource{
			Profile:       profile,
			Current:       current,
			Verified:      verified,
			Open:          profile.ProfileID == requested && notice != "active",
			ConfigVersion: p.Status.ConfigVersion,
			CanTest:       profile.ProviderKind != "openai_account" || connectedAccounts[profile.AuthConnectionID],
		}
		if row.ProfileID == requested {
			v.TargetModel = row.ProfileID
			switch notice {
			case "tested":
				if !row.Verified {
					p.Notice = ""
				}
			case "active":
				if !row.Current {
					p.Notice = ""
				}
			}
		}
		if models, known := v.AccountModels[profile.AuthConnectionID]; known {
			v.AccountModels[profile.AuthConnectionID] = append(models, row)
		} else {
			v.DirectModels = append(v.DirectModels, row)
		}
	}
	if requested != "" && v.TargetModel == "" {
		return errPageNotFound
	}
	return nil
}

var errPageNotFound = errors.New("page not found")

// A successful probe is not activation: candidates remain unverified in their
// profile row until Select commits. Presentation reads existing proof metadata;
// Control.Select still validates live credentials and every frozen version.
func (s *Server) profileVerified(ctx context.Context, p llm.Profile) bool {
	if s.Control.Store == nil || s.Control.BridgeIdentity == "" || s.Control.BridgeError != nil {
		return false
	}
	record, err := s.Control.Store.Probe(ctx, p.ProfileID)
	if err != nil || record.ProfileVersion != p.Version || record.BridgeIdentity != s.Control.BridgeIdentity || record.Result.ValidateCapabilities(p.Capabilities) != nil {
		return false
	}
	version := int64(0)
	if p.AuthConnectionID != "" {
		a, e := s.Control.Store.Auth(ctx, p.AuthConnectionID)
		if e != nil || a.Status != "connected" ||
			a.AuthKind != "chatgpt_oauth" && a.ExpiresAt != nil && !a.ExpiresAt.After(time.Now()) {
			return false
		}
		version = a.Version
	}
	return record.AuthVersion == version
}
