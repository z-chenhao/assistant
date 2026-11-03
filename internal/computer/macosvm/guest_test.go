package macosvm_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/collie/internal/computer"
	"github.com/z-chenhao/collie/internal/computer/macosvm"
	"github.com/z-chenhao/collie/internal/computer/macosvm/fixture"
	"github.com/z-chenhao/collie/internal/llm/secret"
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
	t.Setenv("COLLIE_COMPUTER_FIXTURE_PASSWORD", "fixture-only")
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

func TestReadOnlyPreviewCapturesExistingWindowAndNeverStartsMissingComputer(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	peer, err := fixture.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	t.Setenv("COLLIE_COMPUTER_FIXTURE_PASSWORD", "fixture-only")
	source, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(source, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"disk.img", "nvram.bin", "config.json"} {
		data := []byte("fixture-" + name)
		if name == "config.json" {
			data = []byte(`{"macAddress":"02:00:00:00:00:01","machineIdentifier":"fixture-base","cpuCount":4}`)
		}
		if err = os.WriteFile(filepath.Join(source, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err = macosvm.ProvisionBase(context.Background(), root, source, peer.Config.Compatibility); err != nil {
		t.Fatal(err)
	}
	provider, err := macosvm.New(peer.Config, macosvm.SSHGuest{Config: peer.Config.Guest, Secrets: secret.Store{}})
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	instance := computer.Instance{ComputerID: "preview-computer", Epoch: 1, NativeName: "collie-preview-window"}
	if _, err = provider.PreviewCapture(ctx, instance); err == nil {
		t.Fatal("preview started a missing Computer")
	}
	request := computer.Request{Instance: instance, OperationID: "preview-provision", Compatibility: peer.Config.Compatibility, Manifest: peer.Config.Manifest, Fence: func(context.Context) error { return nil }}
	if _, err = provider.Ensure(ctx, request); err != nil {
		t.Fatal(err)
	}
	capture, err := provider.PreviewCapture(ctx, instance)
	if err != nil || len(capture.PNG) == 0 || capture.Width < 1 || capture.Height < 1 {
		t.Fatal("existing window preview unavailable", err, capture.Width, capture.Height)
	}
}
