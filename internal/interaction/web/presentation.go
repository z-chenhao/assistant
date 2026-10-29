package web

import (
	"github.com/z-chenhao/erii/internal/interaction"
	"github.com/z-chenhao/erii/internal/llm"
	"strings"
	"time"
)

func activeProfile(s Status) string {
	if s.ActiveModelProfileID == nil {
		return ""
	}
	return *s.ActiveModelProfileID
}
func canSend(s Status) bool {
	return s.Readiness == "ready" || s.Readiness == "" && (s.Status == "idle" || s.Status == "busy")
}
func providerName(kind string) string {
	if kind == "openai_account" {
		return "ChatGPT"
	}
	return "Compatible API"
}
func bindingTarget(b interaction.Binding) string { return b.Channel + ":" + b.ID }
func humanTime(raw string) string {
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return raw
	}
	return t.UTC().Format("Jan 2, 2006 · 15:04 UTC")
}
func toolLabel(name string) string {
	if name == "account_spend" {
		return "Continue model spending beyond the account boundary"
	}
	parts := strings.Split(strings.TrimPrefix(name, "builtin__"), "__")
	if len(parts) == 3 && parts[0] == "mcp" {
		return strings.ReplaceAll(parts[1], "_", " ") + " · " + strings.ReplaceAll(parts[2], "_", " ")
	}
	return strings.ReplaceAll(strings.TrimPrefix(name, "builtin__"), "_", " ")
}
func historyLabel(kind string) string {
	switch kind {
	case "instruction":
		return "Request updated"
	case "completed":
		return "Work round completed"
	case "failed":
		return "Work attempt failed"
	case "needs_input":
		return "Input requested"
	case "unknown":
		return "Outcome unconfirmed"
	}
	return kind
}

type viewOption struct{ Value, Label string }

func taskViews() []viewOption {
	return []viewOption{{"open", "Open"}, {"attention", "Needs me"}, {"closed", "Finished"}, {"all", "All tasks"}}
}
func tabLabel(name string) string {
	switch name {
	case "overview":
		return "Overview"
	case "context":
		return "Context"
	case "wire":
		return "Requests"
	case "diff":
		return "Compare"
	}
	return name
}

func otherProfiles(profiles []llm.Profile, active string) []llm.Profile {
	out := []llm.Profile{}
	for _, p := range profiles {
		if p.ProfileID != active {
			out = append(out, p)
		}
	}
	return out
}
