package web

import (
	"net/http"
	"strconv"

	"github.com/z-chenhao/erii/internal/llm"
)

func (s *Server) addModel(r *http.Request) error {
	input := llm.ProfileInput{Name: r.FormValue("model"), Endpoint: r.FormValue("endpoint"), ModelID: r.FormValue("model"), ProviderKind: "openai_compatible", WireAPI: "openai_completions", ContextWindow: 8192, MaxOutputTokens: 1024, Compatibility: llm.Compatibility{ToolResultFormat: "openai"}, Capabilities: llm.Capabilities{TextInput: true}}
	if r.FormValue("account") != "" {
		input.AuthConnectionID = r.FormValue("account")
		input.ProviderKind = "openai_account"
		input.WireAPI = "openai_responses"
		input.Endpoint = ""
		if s.AccountCatalog == nil {
			return llm.ErrNoBridge
		}
		catalog, err := s.AccountCatalog(r.Context(), input.AuthConnectionID)
		if err != nil {
			return err
		}
		found := false
		for _, m := range catalog {
			if m.ModelID == input.ModelID {
				input.ContextWindow = m.ContextWindow
				input.MaxOutputTokens = min(m.MaxOutputTokens, m.ContextWindow)
				found = true
			}
		}
		if !found {
			return llm.ModelIncompatible
		}
	}
	if _, e := s.Models.AddProfile(r.Context(), input); e != nil {
		return e
	}

	return nil
}
func (s *Server) selectModel(r *http.Request) error {
	if r.FormValue("consent") != "yes" {
		return llm.ErrConflict
	}
	pv, e := strconv.ParseInt(r.FormValue("profile_version"), 10, 64)
	if e != nil {
		return e
	}
	cv, e := strconv.ParseInt(r.FormValue("config_version"), 10, 64)
	if e != nil {
		return e
	}
	if e = s.Control.Select(r.Context(), r.FormValue("profile"), pv, cv); e != nil {
		return e
	}
	if e = s.Core.WaitForRuntime(r.Context()); e != nil {
		return e
	}

	return nil
}
