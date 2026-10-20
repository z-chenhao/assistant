package macosvm_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/computer"
	"github.com/z-chenhao/erii/internal/computer/macosvm"
	"github.com/z-chenhao/erii/internal/computer/macosvm/fixture"
	"github.com/z-chenhao/erii/internal/llm/secret"
)

func TestSSHNegotiationFrozenIdentityAndCompatibility(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := fixture.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	t.Setenv("ERII_COMPUTER_FIXTURE_PASSWORD", "fixture-only")
	g := macosvm.SSHGuest{Config: s.Config.Guest, Secrets: secret.Store{}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := g.Ready(ctx, "127.0.0.1", s.Config.Compatibility); err != nil {
		t.Fatal("pinned Ed25519 with ECDSA also offered", err)
	}
	wrong := s.Config.Compatibility
	wrong.DriverSHA256 = strings.Repeat("b", 64)
	if err := g.Ready(ctx, "127.0.0.1", wrong); !errors.Is(err, computer.ErrCompatibility) {
		t.Fatal("wrong Driver identity accepted or retried", err)
	}
	g.Config.HostKeySHA256 = "SHA256:" + strings.Repeat("b", 43)
	if err := g.Ready(ctx, "127.0.0.1", s.Config.Compatibility); !errors.Is(err, computer.ErrCompatibility) {
		t.Fatal("wrong Guest key accepted or retried", err)
	}
}
