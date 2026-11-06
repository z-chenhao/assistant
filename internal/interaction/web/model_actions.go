package web

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/z-chenhao/assistant/internal/llm"
)

func (s *Server) showAccountCatalog(w http.ResponseWriter, r *http.Request) error {
	account := r.FormValue("account")
	if account == "" || s.AccountCatalog == nil {
		return llm.ErrConflict
	}
	catalog, catalogErr := s.AccountCatalog(r.Context(), account)
	next := r.Clone(r.Context())
	next.URL.Path = "/settings/models/new"
	next.URL.RawQuery = url.Values{"provider": {"chatgpt"}, "account": {account}}.Encode()
	p, err := s.page(next.Context(), next)
	if err != nil {
		return err
	}
	p.Settings.Catalog = catalog
	p.Settings.CatalogLoaded = true
	p.Settings.CatalogError = catalogErr != nil
	s.render(w, next, p)
	return nil
}

func (s *Server) addModel(r *http.Request) (llm.Profile, error) {
	input := llm.ProfileInput{Name: r.FormValue("model"), Endpoint: r.FormValue("endpoint"), ModelID: r.FormValue("model"), ProviderKind: "openai_compatible", WireAPI: "openai_completions", ContextWindow: 128000, MaxOutputTokens: 16384, Compatibility: llm.Compatibility{ToolResultFormat: "openai"}, Capabilities: llm.Capabilities{TextInput: true, ImageInput: true}}
	if r.FormValue("account") != "" {
		input.AuthConnectionID = r.FormValue("account")
		input.ProviderKind = "openai_account"
		input.WireAPI = "openai_responses"
		input.Endpoint = ""
		if s.AccountCatalog == nil {
			return llm.Profile{}, llm.ErrNoBridge
		}
		catalog, err := s.AccountCatalog(r.Context(), input.AuthConnectionID)
		if err != nil {
			return llm.Profile{}, err
		}
		found := false
		for _, m := range catalog {
			if m.ModelID == input.ModelID {
				input.ContextWindow = m.ContextWindow
				input.MaxOutputTokens = min(m.MaxOutputTokens, m.ContextWindow)
				input.Capabilities.ImageInput = m.ImageInput
				found = true
			}
		}
		if !found {
			return llm.Profile{}, llm.ModelIncompatible
		}
	}
	if name := r.FormValue("name"); name != "" {
		input.Name = name
	}
	return s.Models.AddProfile(r.Context(), input)
}
func (s *Server) testModel(r *http.Request) error {
	ctx := r.Context()
	id := r.FormValue("profile")
	if r.FormValue("restore_current") != "yes" {
		_, err := s.Control.Test(ctx, id)
		return err
	}

	// Testing the current model can restore its existing selection after an
	// auth loss or stale proof. A concurrent switch must win over this restore.
	status, err := s.Core.WebStatus(ctx)
	if err != nil {
		return err
	}
	if activeProfile(status) != id || strconv.FormatInt(status.ConfigVersion, 10) != r.FormValue("config_version") {
		return llm.ErrConflict
	}
	return s.activateModel(r)
}

func (s *Server) selectModel(r *http.Request) error {
	if r.FormValue("consent") != "yes" {
		return llm.ErrConflict
	}
	return s.activateModel(r)
}

func (s *Server) activateModel(r *http.Request) error {
	pv, e := strconv.ParseInt(r.FormValue("profile_version"), 10, 64)
	if e != nil {
		return e
	}
	cv, e := strconv.ParseInt(r.FormValue("config_version"), 10, 64)
	if e != nil {
		return e
	}
	if e = s.Control.SelectWithProbe(r.Context(), r.FormValue("profile"), pv, cv); e != nil {
		return e
	}
	if e = s.Core.WaitForRuntime(r.Context()); e != nil {
		return e
	}

	return nil
}
