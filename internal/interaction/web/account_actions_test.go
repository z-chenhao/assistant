package web

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/llm/account"
)

type loginStore struct{ account.Store }

func (loginStore) DeviceID(context.Context) (string, error) { return "owned-fixture-device", nil }

type loginProvider struct {
	account.Provider
	fail bool
}

func (p loginProvider) AccountStart(context.Context, string, *llm.AccountRegistration, *llm.OAuthCredential) (llm.AccountLogin, error) {
	if p.fail {
		return llm.AccountLogin{}, llm.AuthRequired
	}
	return llm.AccountLogin{Status: "connected", Credential: &llm.OAuthCredential{
		ClientID: "fixture", SubjectHash: strings.Repeat("a", 64), AccessToken: "private-fixture-access", RefreshToken: "private-fixture-refresh", ExpiresAt: time.Now().Add(time.Hour),
	}}, nil
}
func (loginProvider) AccountCancel(context.Context) error { return nil }

type unavailableLoginSecrets struct{ fakeSecrets }

func (unavailableLoginSecrets) Put(context.Context, string, []byte) error {
	return fmt.Errorf("private-backend-diagnostic: %w", llm.ErrSecretStore)
}

func TestAccountStoreFailureExplainsRepairWithoutExposingBackendDetails(t *testing.T) {
	s := &Server{Accounts: &account.Manager{Store: loginStore{}, Provider: loginProvider{}, Secrets: unavailableLoginSecrets{}, LockRoot: t.TempDir()}}
	login := func() {
		t.Helper()
		if err := s.startLogin(httptest.NewRequest("POST", "/auth/login", nil)); err != nil {
			t.Fatal(err)
		}
		select {
		case <-s.authDone:
		case <-time.After(3 * time.Second):
			t.Fatal("login did not complete")
		}
	}
	login()
	if s.auth.Status != "reauth_required" || s.authError != credentialStoreHelp {
		t.Fatal("credential failure did not explain repair")
	}
	for _, language := range []string{"en", "zh-CN"} {
		var out bytes.Buffer
		if err := localizedLayout(language).ExecuteTemplate(&out, "signin", page{Auth: s.auth, AuthError: s.authError}); err != nil {
			t.Fatal(err)
		}
		text := out.String()
		if !strings.Contains(text, translate(language, credentialStoreHelp)) || strings.Contains(text, "private-") || strings.Contains(text, "Account connected.") {
			t.Fatal("credential repair UI missing or exposed private data")
		}
	}
	s.Accounts.Provider = loginProvider{fail: true}
	login()
	if s.auth.Status != "reauth_required" || s.authError != "" {
		t.Fatal("new authentication failure retained an unrelated credential error")
	}
}
