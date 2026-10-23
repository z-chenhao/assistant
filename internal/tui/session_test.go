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
	"github.com/z-chenhao/erii/internal/llm"
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

func TestOversizeInputRejectsWholeLineAndRecoversWithoutEcho(t *testing.T) {
	var out bytes.Buffer
	err := (Session{State: setupState{}}).Run(context.Background(), io.NopCloser(strings.NewReader("/cancel\n"+strings.Repeat("PRIVATE_INPUT", 2000)+"\n/status\n/quit\n")), &out)
	if err != nil || strings.Contains(out.String(), "PRIVATE_INPUT") || !strings.Contains(out.String(), "Input rejected: line exceeds 16 KiB") || strings.Count(out.String(), "State: setup_required") != 2 {
		t.Fatalf("unsafe result: %v %s", err, &out)
	}
}

func TestLineSizeBoundary(t *testing.T) {
	for _, size := range []int{16384, 16385} {
		input := "/cancel\n" + strings.Repeat("x", size) + "\n/quit\n"
		var out bytes.Buffer
		err := (Session{State: setupState{}}).Run(context.Background(), io.NopCloser(strings.NewReader(input)), &out)
		if err != nil || strings.Contains(out.String(), "Input rejected:") != (size > maxLineBytes) {
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

func TestModelPicker(t *testing.T) {
	catalog := []llm.CatalogModel{{ModelID: "first"}, {ModelID: "second"}}
	for _, tc := range []struct {
		name    string
		catalog []llm.CatalogModel
		input   []string
		manual  bool
		want    string
	}{
		{"number", catalog, []string{"2"}, false, "second"},
		{"id", catalog, []string{"first"}, false, "first"},
		{"single Enter", catalog[:1], []string{""}, true, "first"},
		{"manual", nil, []string{"", "my-local-model"}, true, "my-local-model"},
		{"account rejects hidden and blank", catalog, []string{"hidden", "", "3", "1"}, false, "first"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := make(chan inputEvent, len(tc.input))
			for _, text := range tc.input {
				lines <- inputEvent{kind: inputLine, text: text}
			}
			close(lines)
			var out bytes.Buffer
			u := terminal{lines: lines, out: &out}
			got, err := u.chooseModel(context.Background(), tc.catalog, tc.manual)
			if err != nil || got != tc.want {
				t.Fatalf("got %q %v; %s", got, err, &out)
			}
		})
	}
}

func TestProbeFailureRecoveryIsPrintedOnceAndSanitized(t *testing.T) {
	var out bytes.Buffer
	u := terminal{out: &out}
	u.report(&testFailure{id: "profile-id", cause: &llm.ProbeError{Stage: llm.ProbeConnection, Timeout: llm.ProbeStepTimeout, Cause: errors.Join(llm.GenerationCancelled, errors.New("PRIVATE_RESPONSE"))}})
	text := out.String()
	for _, want := range []string{"text and streaming: timed out after 25s", "/test profile-id", "/select profile-id", "/setup"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	if strings.Count(text, "Connection test failed:") != 1 || strings.Contains(text, "PRIVATE_RESPONSE") || strings.Contains(text, "GENERATION_CANCELLED") {
		t.Fatal(text)
	}
}
