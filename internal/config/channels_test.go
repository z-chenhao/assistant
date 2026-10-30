package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/z-chenhao/collie/internal/interaction/feishu"
)

func TestChannelConfigRejectsPublicListenerLiteralSecretsAndUntrustedAPI(t *testing.T) {
	for _, extra := range []string{
		"web:\n  listen: 0.0.0.0:8080\n  token_ref: env:WEB_TOKEN\n",
		"web:\n  listen: localhost:8080\n  token_ref: env:WEB_TOKEN\n",
		"web:\n  listen: 127.0.0.1:8080\n  token_ref: pasted-secret-value\n",
		"feishu:\n  app_id: app\n  tenant_key: tenant\n  verification_token_ref: env:VERIFY\n  encrypt_key_ref: env:ENCRYPT\n  access_token_ref: env:ACCESS\n  api_base: https://untrusted.example\n",
	} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte("version: 1\n"+extra), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path, func(string) (string, bool) { return "", false }); err == nil {
			t.Fatal("unsafe channel config accepted")
		}
	}
}

func TestFeishuExplicitCredentialMode(t *testing.T) {
	base := feishu.Config{AppID: "app", TenantKey: "tenant", VerificationTokenRef: "env:VERIFY", EncryptKeyRef: "env:ENCRYPT"}
	for _, test := range []struct {
		access, secret string
		valid          bool
	}{
		{"env:ACCESS", "", true}, {"", "credential:feishu-app-secret", true}, {"", "", false},
		{"env:ACCESS", "env:SECRET", false}, {"", "plain-secret", false}, {"", "env:BAD NAME", false},
	} {
		cfg := base
		cfg.AccessTokenRef = test.access
		cfg.AppSecretRef = test.secret
		if err := cfg.Validate(); (err == nil) != test.valid {
			t.Fatalf("credential mode valid=%v: %v", test.valid, err)
		}
	}
}
