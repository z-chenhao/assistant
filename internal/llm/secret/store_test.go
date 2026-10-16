package secret

import (
	"context"
	"github.com/z-chenhao/erii/internal/llm"
	"testing"
)

func TestExplicitEnvironmentSecretAndInvalidReference(t *testing.T) {
	t.Setenv("ERII_SECRET_FIXTURE", "synthetic")
	s := Store{}
	value, err := s.Get(context.Background(), "env:ERII_SECRET_FIXTURE")
	if err != nil || string(value) != "synthetic" {
		t.Fatal("explicit reference failed")
	}
	for _, name := range []string{"", "1BAD", "KEY\nVALUE", "KEY=VALUE", "../file"} {
		if ValidName(name) {
			t.Fatal("invalid environment name")
		}
		if _, err := s.Get(context.Background(), "env:"+name); err != llm.ErrSecretStore {
			t.Fatal("unsafe reference error")
		}
	}
}
