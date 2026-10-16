package tui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/conversation"
)

type setupState struct{}

func (setupState) Diagnostics(context.Context) (agent.Diagnostics, error) {
	return agent.Diagnostics{Readiness: agent.SetupRequired}, nil
}
func (setupState) History(context.Context) ([]conversation.Message, error) { return nil, nil }

func TestSetupCancelAndIngressGate(t *testing.T) {
	var out bytes.Buffer
	err := (Session{State: setupState{}}).Run(context.Background(), io.NopCloser(strings.NewReader("/cancel\nhello\n/status\n/quit\n")), &out)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Choose model connection", "Setup cancelled.", "State: setup_required.", "Use /setup to repair"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q: %s", want, &out)
		}
	}
}

func TestPromptCancellationClosesAndJoinsReader(t *testing.T) {
	input, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- (Session{State: setupState{}}).Run(ctx, input, io.Discard) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("input reader survived cancellation")
	}
}

func TestOversizeInputFailsWithoutEcho(t *testing.T) {
	var out bytes.Buffer
	err := (Session{State: setupState{}}).Run(context.Background(), io.NopCloser(strings.NewReader(strings.Repeat("PRIVATE_INPUT", 2000))), &out)
	if err == nil || strings.Contains(out.String()+err.Error(), "PRIVATE_INPUT") {
		t.Fatalf("unsafe result: %v %s", err, &out)
	}
}

func TestLineSizeBoundary(t *testing.T) {
	for _, size := range []int{16384, 16385} {
		input := "/cancel\n" + strings.Repeat("x", size) + "\n/quit\n"
		err := (Session{State: setupState{}}).Run(context.Background(), io.NopCloser(strings.NewReader(input)), io.Discard)
		if (err == nil) != (size == 16384) {
			t.Fatalf("size %d: %v", size, err)
		}
	}
}

func TestUntrustedTextAndErrorNeverExecuteTerminalControls(t *testing.T) {
	var out bytes.Buffer
	u := terminal{out: &out}
	u.print("\x1b]52;c;PRIVATE\a\x1b[2J\rtext\u202e")
	u.report(errors.New("PRIVATE_PROVIDER_BODY"))
	text := out.String()
	if strings.ContainsAny(text, "\x1b\a\r\u202e") || strings.Contains(text, "PRIVATE_PROVIDER_BODY") {
		t.Fatal(text)
	}
}
