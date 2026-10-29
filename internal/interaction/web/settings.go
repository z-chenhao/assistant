package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/z-chenhao/collie/internal/llm"
)

type settingsView struct {
	Verified              map[string]bool
	TargetUnavailable     bool
	Section               string
	Provider              string
	Profile               *llm.Profile
	Account               *llm.AuthConnection
	Catalog               []llm.CatalogModel
	CatalogError          bool
	Related               []llm.Profile
	Target                string
	Model, Endpoint, Name string
	ImageInput            bool
}

func modelURL(id string) string { return "/settings/models/" + url.PathEscape(id) }
func (s *Server) loadSettings(ctx context.Context, r *http.Request, p *page) error {
	v := &settingsView{Verified: map[string]bool{}, Section: "models", Provider: r.URL.Query().Get("provider"), Endpoint: "http://127.0.0.1:8000/v1"}
	p.Settings = v
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
	var err error
	p.Profiles, err = s.Models.ListProfiles(ctx)
	if err != nil {
		return err
	}
	for _, profile := range p.Profiles {
		v.Verified[profile.ProfileID] = s.profileVerified(ctx, profile)
	}
	if r.URL.Path == "/settings/models/new" {
		v.Section = "new"
		if v.Provider != "" && v.Provider != "compatible" && v.Provider != "chatgpt" {
			return llm.ErrConflict
		}
	} else if strings.HasPrefix(r.URL.Path, "/settings/models/") {
		v.Section = "model"
		id := strings.TrimPrefix(r.URL.Path, "/settings/models/")
		for i := range p.Profiles {
			if p.Profiles[i].ProfileID == id {
				v.Profile = &p.Profiles[i]
				break
			}
		}
		if v.Profile == nil {
			return errPageNotFound
		}
	}
	// Flash text cannot claim stale verification or activation after the profile,
	// credentials or bridge have changed since the redirect.
	if r.URL.Query().Get("notice") == "tested" && (v.Profile == nil || !v.Verified[v.Profile.ProfileID]) {
		p.Notice = ""
	}
	if r.URL.Query().Get("notice") == "active" && (v.Profile == nil || v.Profile.ProfileID != activeProfile(p.Status)) {
		p.Notice = ""
	}
	// Account lifecycle data is only needed inside the add/detail workflow.
	if (v.Section == "new" && v.Provider == "chatgpt" || v.Profile != nil && v.Profile.AuthConnectionID != "") && s.Accounts != nil {
		p.Accounts, err = s.Accounts.Store.Accounts(ctx)
		if err != nil {
			return err
		}
		account := r.URL.Query().Get("account")
		if v.Profile != nil {
			account = v.Profile.AuthConnectionID
		}
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
		if v.Account != nil {
			for _, profile := range p.Profiles {
				if profile.AuthConnectionID == v.Account.ConnectionID {
					v.Related = append(v.Related, profile)
				}
			}
			if v.Section == "new" && v.Account.Status == "connected" && s.AccountCatalog != nil {
				v.Catalog, err = s.AccountCatalog(ctx, v.Account.ConnectionID)
				v.CatalogError = err != nil
			}
		}
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
		if e != nil || a.Status != "connected" || a.ExpiresAt != nil && !a.ExpiresAt.After(time.Now()) {
			return false
		}
		version = a.Version
	}
	return record.AuthVersion == version
}
