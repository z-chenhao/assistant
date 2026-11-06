package app

import (
	"context"
	"os"
	"sync/atomic"
	"testing"

	"github.com/z-chenhao/assistant/internal/config"
	"github.com/z-chenhao/assistant/internal/conversation"
	"github.com/z-chenhao/assistant/internal/interaction"
	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/llm/bridge"
	"github.com/z-chenhao/assistant/internal/llm/fake"
)

type admissionSecrets struct{ reads atomic.Int32 }

func (s *admissionSecrets) Get(context.Context, string) ([]byte, error) {
	s.reads.Add(1)
	return nil, llm.ErrSecretStore
}
func (s *admissionSecrets) Put(context.Context, string, []byte) error { return llm.ErrSecretStore }
func (s *admissionSecrets) Delete(context.Context, string) error      { return llm.ErrSecretStore }

func TestInboundAdmissionDoesNotWaitForCredentialIO(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	cfg, err := config.Load("", func(k string) (string, bool) { return root, k == "STEWARD_DATA_ROOT" })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Defaults = &config.Defaults{}
	a, err := OpenWithTestBridge(ctx, cfg, "core")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	provider, err := bridge.Open(ctx, os.Args[0], "-test.run=^TestAdmissionProviderProcess$", "--", "admission-provider")
	if err != nil {
		t.Fatal(err)
	}
	a.provider = provider
	a.control.Probe = provider
	a.control.BridgeIdentity = "admission-fixture-v1"
	a.control.BridgeError = nil
	auth, err := a.LoginKey(ctx, []byte("synthetic-key"))
	if err != nil {
		t.Fatal(err)
	}
	input := llm.ProfileInput{Name: "admission fixture", ProviderKind: "openai_compatible", WireAPI: "openai_completions", Endpoint: "http://127.0.0.1:1/v1", ModelID: "fixture-model", AuthConnectionID: auth.ConnectionID, ContextWindow: 32768, MaxOutputTokens: 4096, Compatibility: llm.Compatibility{ToolResultFormat: "openai"}, Capabilities: llm.Capabilities{TextInput: true, ToolCalls: true, Streaming: true}}
	p, err := a.Models().AddProfile(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Models().RecordProbe(ctx, p, llm.ProbeRecord{ProfileVersion: p.Version, AuthVersion: auth.Version, BridgeIdentity: a.control.BridgeIdentity, Result: llm.ProbeResult{Streaming: true, ToolCalls: true, Continuation: true, Cancellation: true, Clipping: true}}); err != nil {
		t.Fatal(err)
	}
	d, err := a.Diagnostics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.control.Select(ctx, p.ProfileID, p.Version, d.ConfigVersion); err != nil {
		t.Fatal(err)
	}
	if err := a.CheckChannel(ctx, "web", "browser", "local"); err != nil {
		if _, err := a.BindChannel(ctx, "web", "browser", "local", true, 0); err != nil {
			t.Fatal(err)
		}
	}
	secrets := &admissionSecrets{}
	a.control.Secrets = secrets
	receipt, err := a.AcceptInbound(ctx, interaction.InboundEvent{Channel: "web", Subject: "browser", Destination: "local", EventID: "fast-admission", SourceID: "fast-admission", Content: conversation.Text("Keep my request while provider credentials are prepared.")})
	if err != nil || receipt.MessageID == "" {
		t.Fatal("durable admission waited on provider credentials", err)
	}
	if secrets.reads.Load() != 0 {
		t.Fatal("HTTP admission read provider credentials")
	}
	// Execution readiness still checks the real credential boundary.
	if a.Actor(nil).Ready(ctx) {
		t.Fatal("missing credentials incorrectly allowed model execution")
	}
	if secrets.reads.Load() == 0 {
		t.Fatal("execution skipped credential validation")
	}
}

func TestAdmissionProviderProcess(t *testing.T) {
	if os.Args[len(os.Args)-1] != "admission-provider" {
		return
	}
	if err := fake.Serve(os.Stdin, os.Stdout, "core"); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
