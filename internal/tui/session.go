// Package tui is the local, line-oriented terminal adapter. It owns no Agent
// state and exposes settings only to the human, never as model Tools.
package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/task"
)

// These private-to-the-repository seams are consumed by this adapter; app wires
// the existing control plane and Actor. They do not form another execution path.
type State interface {
	Diagnostics(context.Context) (agent.Diagnostics, error)
	History(context.Context) ([]conversation.Message, error)
}
type Models interface {
	AddProfile(context.Context, llm.ProfileInput) (llm.Profile, error)
	ListProfiles(context.Context) ([]llm.Profile, error)
	Profile(context.Context, string) (llm.Profile, error)
}
type Accounts interface {
	Login(context.Context, string, func(llm.AuthEvent)) (llm.AuthConnection, error)
	Logout(context.Context, string) (bool, error)
}
type Session struct {
	DecideApproval   func(context.Context, string, string, string) error
	Messages         func(context.Context) ([]conversation.Message, error)
	State            State
	Models           Models
	Accounts         Accounts
	Control          llm.Control
	Actor            agent.Actor
	CurrentActor     func() agent.Actor
	AwaitRuntime     func(context.Context) error
	Discover         func(context.Context, string, string) ([]llm.CatalogModel, error)
	AccountCatalog   func(context.Context, string) ([]llm.CatalogModel, error)
	LoginEnvironment func(context.Context, string) (llm.AuthConnection, error)
}

func (s Session) actor() agent.Actor {
	if s.CurrentActor != nil {
		return s.CurrentActor()
	}
	return s.Actor
}

var cancelled = errors.New("setup cancelled")

// Run owns input and closes it on exit, including signal cancellation. The CLI
// supplies stdin; tests supply a disposable pipe. Output stays on this terminal;
// private login URLs are never copied to history, traces or an evidence log.
func (s Session) Run(ctx context.Context, input io.ReadCloser, out io.Writer) (result error) {
	lines := make(chan inputEvent)
	readerDone := make(chan error, 1)
	readCtx, stop := context.WithCancel(ctx)
	defer func() {
		stop()
		closeErr := input.Close()
		if err := <-readerDone; err != nil && ctx.Err() == nil {
			result = errors.New("terminal input failed")
		}
		if ctx.Err() != nil {
			result = nil
		}
		result = errors.Join(result, closeErr)
	}()
	go func() {
		interactive, echo := false, false
		if terminal, ok := input.(*terminalInput); ok && terminal.settings != nil {
			interactive, echo = true, terminal.echoEnabled()
		}
		err := readInput(readCtx, input, interactive, echo, func(event inputEvent) bool {
			select {
			case lines <- event:
				return true
			case <-readCtx.Done():
				return false
			}
		})
		if readCtx.Err() != nil {
			err = nil
		}
		readerDone <- err
		close(lines)
	}()
	u := terminal{Session: s, lines: lines, out: out}
	work, cancelWork := context.WithCancel(ctx)
	u.work = work
	defer func() {
		cancelWork()
		if u.workDone != nil {
			<-u.workDone
		}
	}()
	u.print("Erii — /setup /models /test ID /select ID /login [ACCOUNT_ID] /logout ACCOUNT_ID /history /status /approve ID HASH /deny ID HASH /quit")
	d, err := s.State.Diagnostics(ctx)
	if err != nil {
		return err
	}
	u.print("State: " + d.Readiness)
	if d.Readiness == agent.SetupRequired {
		u.report(u.setup(ctx))
	}
	if ctx.Err() == nil {
		// Reclaim durable Main work before accepting a new line after a restart.
		if d, err = s.State.Diagnostics(ctx); err != nil {
			return err
		}
		if d.AgentIngressEnabled {
			u.startDrain()
		}
		u.report(u.history(ctx))
	}
	for ctx.Err() == nil {
		line, err := u.read(ctx, "You> ")
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if line == "" {
			continue
		}
		if line == "/quit" {
			return nil
		}
		if strings.HasPrefix(line, "/") {
			u.report(u.command(ctx, line))
			continue
		}
		d, err := s.State.Diagnostics(ctx)
		if err != nil {
			return err
		}
		if !d.AgentIngressEnabled {
			u.print("State: " + d.Readiness + ". Use /setup to repair the connection; history remains available.")
			continue
		}
		_, err = s.actor().Submit(ctx, uuid.Must(uuid.NewV7()).String(), line)
		if err == nil {
			u.print("Erii: busy")
			u.startDrain()
		}
		u.report(err)
	}
	return nil
}

type terminal struct {
	Session
	lines        <-chan inputEvent
	out          io.Writer
	seenMessages map[string]bool
	work         context.Context
	workDone     chan error
	drainAgain   bool
	updates      chan struct{}
}

// Keep accepting input while Main is busy. Actor.Submit routes Steering to the
// same durable Main; this adapter never starts a second concurrent Drain.
func (u *terminal) startDrain() {
	if u.workDone != nil {
		u.drainAgain = true
		return
	}
	done := make(chan error, 1)
	u.workDone = done
	if u.updates == nil {
		u.updates = make(chan struct{}, 1)
	}
	actor := u.actor()
	actor.Sink = func(e agent.Event) {
		if e.RunKind == "main" && (e.Kind == "turn_end" || e.Kind == "agent_end") {
			select {
			case u.updates <- struct{}{}:
			default:
			}
		}
	}
	go func() { done <- actor.Drain(u.work) }()
}

// Provider and model text are untrusted terminal data. Preserve plain text,
// never ANSI/OSC controls (including pasted control sequences in user input).
func plain(text string) string {
	return strings.Map(func(r rune) rune {
		if r != '\n' && r != '\t' && (unicode.IsControl(r) || unicode.In(r, unicode.Cf)) {
			return -1
		}
		return r
	}, text)
}
func (u *terminal) print(text string) { fmt.Fprintln(u.out, plain(text)) }
func (u *terminal) read(ctx context.Context, prompt string) (string, error) {
	fmt.Fprint(u.out, prompt)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			if u.Messages != nil {
				u.report(u.history(ctx))
			}
		case <-u.updates:
			u.report(u.history(ctx))
			fmt.Fprint(u.out, prompt)
		case err := <-u.workDone:
			u.workDone = nil
			u.report(err)
			u.report(u.history(ctx))
			if u.drainAgain {
				u.drainAgain = false
				u.startDrain()
			} else {
				u.print("Erii: idle")
			}
			fmt.Fprint(u.out, prompt)
		case event, ok := <-u.lines:
			if !ok {
				return "", io.EOF
			}
			switch event.kind {
			case inputEcho:
				fmt.Fprint(u.out, event.text)
			case inputRedraw:
				fmt.Fprint(u.out, "\r\x1b[2K", prompt, plain(event.text))
			case inputRejected:
				u.print("Input rejected: " + event.err.Error())
				fmt.Fprint(u.out, prompt)
			case inputLine:
				return strings.TrimSpace(event.text), nil
			}
		}
	}
}
func (u *terminal) ask(ctx context.Context, prompt string) (string, error) {
	line, err := u.read(ctx, prompt+" (/cancel to cancel)> ")
	if line == "/cancel" {
		return "", cancelled
	}
	return line, err
}
func (u *terminal) report(err error) {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
		return
	}
	if errors.Is(err, cancelled) {
		u.print("Setup cancelled. Active model unchanged.")
		return
	}
	if errors.Is(err, llm.ErrNoBridge) {
		u.print("Provider Bridge is not configured. Check the connection settings.")
		return
	}
	// Do not render arbitrary error strings, HTTP bodies, endpoints or input.
	for _, known := range []error{llm.AuthRequired, llm.ModelIncompatible, llm.ProviderUnavailable, llm.RateLimited, llm.GenerationCancelled, llm.ErrConflict, llm.ErrUnverified, llm.ErrSecretStore, llm.ErrSecretCleanup} {
		if errors.Is(err, known) {
			u.print("Operation failed: " + known.Error())
			return
		}
	}
	u.print("Operation failed. Check the connection and model settings, then retry explicitly.")
}
func (u *terminal) history(ctx context.Context) error {
	read := u.State.History
	if u.Messages != nil {
		read = u.Messages
	}
	messages, err := read(ctx)
	if err != nil {
		return err
	}
	if u.seenMessages == nil {
		u.seenMessages = map[string]bool{}
	}
	for _, m := range messages {
		if u.seenMessages[m.MessageID] {
			continue
		}
		for _, p := range m.Content {
			if p.Type == "text" {
				u.print(m.Role + ": " + p.Text)
			} else if p.ArtifactRef != "" {
				u.print(m.Role + " attachment: " + p.ArtifactRef)
			} else {
				u.print(m.Role + ": [" + p.Type + "]")
			}
		}
		u.seenMessages[m.MessageID] = true
	}
	return nil
}
func (u *terminal) authEvent(e llm.AuthEvent) {
	switch e.Kind {
	case "open_url":
		u.print("Open this private login URL in your browser (Ctrl-C cancels login): " + e.URL)
	case "device_code":
		u.print("Login code: " + e.DeviceCode)
	case "progress":
		u.print("Login in progress…")
	case "choose_workspace":
		for _, choice := range e.Choices {
			u.print("Workspace: " + choice)
		}
	case "input_required":
		u.print("Complete the login prompt in your browser.")
	}
}
func (u *terminal) command(ctx context.Context, line string) error {
	args := strings.Fields(line)
	switch {
	case len(args) == 3 && (args[0] == "/approve" || args[0] == "/deny"):
		if u.DecideApproval == nil {
			return agent.ErrConflict
		}
		decision := "approved"
		if args[0] == "/deny" {
			decision = "denied"
		}
		if err := u.DecideApproval(ctx, args[1], args[2], decision); err != nil {
			return err
		}
		u.print("Approval decision saved.")
		u.startDrain()
		return nil
	case line == "/setup":
		return u.setup(ctx)
	case line == "/history":
		u.seenMessages = nil
		return u.history(ctx)
	case line == "/status":
		d, err := u.State.Diagnostics(ctx)
		if err == nil {
			u.print("State: " + d.Readiness)
			if activity, ok := u.State.(interface {
				Activity(context.Context) ([]task.Summary, error)
			}); ok {
				items, readErr := activity.Activity(ctx)
				if readErr != nil {
					return readErr
				}
				for _, item := range items {
					u.print(item.Title + ": " + item.Status + " " + item.ProgressSummary)
				}
			}
		}
		return err
	case line == "/models":
		profiles, err := u.Models.ListProfiles(ctx)
		if err != nil {
			return err
		}
		for _, p := range profiles {
			u.print(p.ProfileID + " " + p.ModelID + " " + p.Status)
		}
		return nil
	case len(args) == 2 && args[0] == "/test":
		_, err := u.Control.Test(ctx, args[1])
		if err == nil {
			u.print("Probe passed.")
		}
		return err
	case len(args) == 2 && args[0] == "/select":
		return u.selectModel(ctx, args[1])
	case len(args) <= 2 && args[0] == "/login":
		id := ""
		if len(args) == 2 {
			id = args[1]
		}
		a, err := u.Accounts.Login(ctx, id, u.authEvent)
		if err == nil {
			u.print("Account connected: " + a.ConnectionID)
		}
		return err
	case len(args) == 2 && args[0] == "/logout":
		_, err := u.Accounts.Logout(ctx, args[1])
		if err == nil {
			u.print("Account logged out.")
		}
		return err
	default:
		u.print("Unknown command. Use /setup /models /test ID /select ID /login [ACCOUNT_ID] /logout ACCOUNT_ID /history /status /approve ID HASH /deny ID HASH /quit")
		return nil
	}
}
func (u *terminal) selectModel(ctx context.Context, id string) error {
	p, err := u.Models.Profile(ctx, id)
	if err != nil {
		return err
	}
	d, err := u.State.Diagnostics(ctx)
	if err != nil {
		return err
	}
	u.print("Privacy: requests and Tool schemas go to the selected provider or local endpoint. localhost is the Go Host. Credentials stay in SecretStore. Erii never switches providers automatically.")
	answer, err := u.ask(ctx, "Activate "+plain(p.ModelID)+" via "+p.ProviderKind+"? Type yes")
	if err != nil {
		return err
	}
	if answer != "yes" {
		return cancelled
	}
	if err := u.Control.Select(ctx, id, p.Version, d.ConfigVersion); err != nil {
		return err
	}
	if u.AwaitRuntime != nil {
		if err := u.AwaitRuntime(ctx); err != nil {
			return err
		}
	}
	u.print("Model selected. New Runs use this model.")
	return nil
}
func (u *terminal) setup(ctx context.Context) error {
	u.print("Choose model connection\n  1. Local / OpenAI-compatible endpoint\n  2. OpenAI — Sign in with ChatGPT")
	choice, err := u.ask(ctx, "Connection [1/2]")
	if err != nil {
		return err
	}
	p := llm.ProfileInput{ProviderKind: "openai_compatible", WireAPI: "openai_completions", ContextWindow: 8192, MaxOutputTokens: 1024, Compatibility: llm.Compatibility{ToolResultFormat: "openai"}, Capabilities: llm.Capabilities{TextInput: true}}
	var catalog []llm.CatalogModel
	switch choice {
	case "1":
		p.Endpoint, err = u.ask(ctx, "Endpoint URL [http://127.0.0.1:8000/v1]")
		if err != nil {
			return err
		}
		if p.Endpoint == "" {
			p.Endpoint = "http://127.0.0.1:8000/v1"
		}
		keyEnv, err := u.ask(ctx, "API key environment variable name [blank for no key; never paste a key]")
		if err != nil {
			return err
		}
		if keyEnv != "" {
			a, err := u.LoginEnvironment(ctx, keyEnv)
			if err != nil {
				return err
			}
			p.AuthConnectionID = a.ConnectionID
		}
		catalog, err = u.Discover(ctx, p.Endpoint, p.AuthConnectionID)
		if err != nil {
			u.print("Catalog unavailable. Enter a model ID manually; Probe is still required.")
		}
	case "2":
		p.ProviderKind, p.WireAPI = "openai_account", "openai_responses"
		id, err := u.ask(ctx, "Account ID [blank for new ChatGPT login]")
		if err != nil {
			return err
		}
		a, err := u.Accounts.Login(ctx, id, u.authEvent)
		if err != nil {
			return err
		}
		p.AuthConnectionID = a.ConnectionID
		u.print("Account connected: " + a.ConnectionID)
		catalog, err = u.AccountCatalog(ctx, a.ConnectionID)
		if err != nil {
			return err
		}
	default:
		return cancelled
	}
	for _, m := range catalog {
		u.print("Model: " + m.ModelID)
	}
	p.ModelID, err = u.ask(ctx, "Model ID")
	if err != nil {
		return err
	}
	p.Name = p.ModelID
	found := false
	for _, m := range catalog {
		if m.ModelID == p.ModelID {
			found = true
			if m.ContextWindow > 0 && m.MaxOutputTokens > 0 {
				p.ContextWindow, p.MaxOutputTokens = m.ContextWindow, min(m.MaxOutputTokens, m.ContextWindow)
			}
		}
	}
	if p.ProviderKind == "openai_account" && !found {
		return llm.ModelNotFound
	}
	if p.ProviderKind == "openai_compatible" {
		// Local catalog IDs carry no verified limits. Ask for explicit bounds;
		// conservative defaults are candidates until the actual Probe succeeds.
		for _, setting := range []struct {
			name  string
			value *int64
		}{{"Context tokens", &p.ContextWindow}, {"Max output tokens", &p.MaxOutputTokens}} {
			text, err := u.ask(ctx, fmt.Sprintf("%s [%d]", setting.name, *setting.value))
			if err != nil {
				return err
			}
			if text != "" {
				n, err := strconv.ParseInt(text, 10, 64)
				if err != nil {
					return llm.ModelIncompatible
				}
				*setting.value = n
			}
		}
	}
	if err := p.ValidateActivation(); err != nil {
		return err
	}
	profile, err := u.Models.AddProfile(ctx, p)
	if err != nil {
		return err
	}
	u.print("Testing connection and Agent capabilities…")
	if _, err = u.Control.Test(ctx, profile.ProfileID); err != nil {
		return err
	}
	u.print("Probe passed: text, streaming, Tool calls, continuation, cancellation and clipping. Additional modalities and reasoning settings are unverified.")
	return u.selectModel(ctx, profile.ProfileID)
}
