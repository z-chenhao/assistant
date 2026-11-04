package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/z-chenhao/assistant/internal/llm"
)

const credentialStoreHelp = "The credential store is unavailable. Unlock your login keyring and try again. On Linux, install libsecret and a Secret Service keyring."

func (s *Server) startLogin(r *http.Request) error {
	if s.Accounts == nil {
		return llm.ErrNoBridge
	}
	s.mu.Lock()
	if s.auth.Status == "pending" {
		s.mu.Unlock()
		return llm.ErrConflict
	}
	lifetime := s.lifetime
	if lifetime == nil {
		lifetime = context.Background()
	}
	ctx, cancel := context.WithCancel(lifetime)
	s.authCancel = cancel
	s.auth = llm.AuthState{Status: "pending", Events: []llm.AuthEvent{}}
	s.authError = ""
	s.accountID = ""
	done := make(chan struct{})
	s.authDone = done
	s.mu.Unlock()
	accountID := r.FormValue("account")
	go func() {
		defer close(done)
		a, e := s.Accounts.Login(ctx, accountID, func(event llm.AuthEvent) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.auth.Events = append(s.auth.Events, event)
		})
		s.mu.Lock()
		defer s.mu.Unlock()
		s.auth.Status = "connected"
		if e != nil {
			s.auth.Status = "reauth_required"
			if errors.Is(e, llm.ErrSecretStore) || errors.Is(e, llm.ErrSecretCleanup) {
				s.authError = credentialStoreHelp
			}
		}
		s.accountID = a.ConnectionID
	}()

	return nil
}
