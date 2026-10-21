package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"net/http"

	"github.com/z-chenhao/erii/internal/delivery/ui"
	"github.com/z-chenhao/erii/internal/interaction"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/task"
)

type shownMessage struct {
	Role       string
	Sequence   int64
	Projection shownProjection
}
type shownProjection struct {
	HTML     template.HTML
	Spec     string
	Fallback string
}
type page struct {
	Accounts   []llm.AuthConnection
	Bindings   []interaction.Binding
	Preference interaction.Preference
	Login      bool
	Tab        string
	Status     Status
	Messages   []shownMessage
	Activity   []task.Summary
	Approvals  []shownProjection
	Profiles   []llm.Profile
	SourceID   string
	Auth       llm.AuthState
	AccountID  string
}

func (s *Server) show(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := page{Tab: r.URL.Path}
	var err error
	p.Status, err = s.Core.WebStatus(ctx)
	if err != nil {
		fail(w, 503)
		return
	}
	if s.Accounts != nil {
		p.Accounts, err = s.Accounts.Store.Accounts(ctx)
		if err != nil {
			fail(w, 503)
			return
		}
	}
	p.Bindings, err = s.Core.ChannelBindings(ctx)
	if err != nil {
		fail(w, 503)
		return
	}
	p.Preference, err = s.Core.ChannelPreference(ctx)
	if err != nil {
		fail(w, 503)
		return
	}
	p.Profiles, err = s.Models.ListProfiles(ctx)
	if err != nil {
		fail(w, 503)
		return
	}
	p.Activity, err = s.Core.Activity(ctx)
	if err != nil {
		fail(w, 503)
		return
	}
	approvals, err := s.Core.Approvals(ctx)
	if err != nil {
		fail(w, 503)
		return
	}
	for _, a := range approvals {
		if a.Status == "pending" {
			spec := ui.Spec{SchemaVersion: ui.Version, Fallback: a.Arguments, Components: []ui.Component{{Type: "confirmation", Text: a.Tool + "\n" + a.Arguments, ApprovalID: a.ID, Hash: a.Hash}}}
			raw, _ := json.Marshal(spec)
			p.Approvals = append(p.Approvals, shownProjection{HTML: ui.HTML(raw, spec.Fallback), Spec: string(raw), Fallback: spec.Fallback})
		}
	}
	messages, err := s.Core.History(ctx)
	if err != nil {
		fail(w, 503)
		return
	}
	for _, m := range messages {
		fallback := ""
		for _, c := range m.Content {
			if c.Type == "text" {
				fallback += c.Text + "\n"
			} else if c.ArtifactRef != "" {
				fallback += c.Type + ": " + c.ArtifactRef + "\n"
			}
		}
		raw, err := s.Core.SaveWebProjection(ctx, m)
		if err != nil {
			fail(w, 503)
			return
		}
		p.Messages = append(p.Messages, shownMessage{Role: m.Role, Sequence: m.Sequence, Projection: shownProjection{HTML: ui.HTML(raw, fallback), Spec: string(raw), Fallback: fallback}})
	}
	key := make([]byte, 16)
	if _, err = rand.Read(key); err != nil {
		fail(w, 500)
		return
	}
	p.SourceID = hex.EncodeToString(key)
	s.mu.Lock()
	p.Auth = s.auth
	p.AccountID = s.accountID
	s.mu.Unlock()
	s.render(w, p)
}
func (s *Server) render(w http.ResponseWriter, p page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := layout.Execute(w, p); err != nil {
		return
	}
}

var layout = template.Must(template.New("page").Funcs(template.FuncMap{"authURL": AuthURL}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Erii · Personal assistant</title><link rel="icon" href="/favicon.ico"><link rel="stylesheet" href="/style.css"><script defer src="/app.js"></script></head><body>
{{if .Login}}<main class="login"><img class="mark" src="/brand/eri-icon-192.png" alt="Erii character logo" width="80" height="80"><p class="eyebrow">YOUR PERSONAL ASSISTANT</p><h1>Welcome to Erii</h1><p>Connect to your private workspace.</p><form action="/session" method="post"><label>Workspace access token<input type="password" name="token" required autocomplete="off"></label><button>Open workspace →</button></form><small>The installation owner configures access through the credential store.</small></main>
{{else}}<aside><a class="brand" href="/"><img src="/brand/eri-icon-192.png" alt="Erii character logo" width="40" height="40"><span>Erii</span></a><p class="eyebrow">WORKSPACE</p><nav><a href="/" {{if eq .Tab "/"}}aria-current="page"{{end}}>◌ &nbsp; Conversation</a><a href="/progress" {{if eq .Tab "/progress"}}aria-current="page"{{end}}>◷ &nbsp; Ongoing work</a><a href="/settings" {{if eq .Tab "/settings"}}aria-current="page"{{end}}>⚙ &nbsp; Settings</a></nav><div class="aside-bottom"><img class="avatar" src="/brand/eri-icon-192.png" alt="Erii default agent avatar" width="36" height="36"><div><strong>Erii</strong><small>Private workspace</small></div><span class="dot"></span></div></aside>
<main><header><span class="page-title">{{if eq .Tab "/progress"}}Ongoing work{{else if eq .Tab "/settings"}}Settings{{else}}Conversation{{end}}</span><span class="state"><span class="dot"></span> {{.Status.Status}}</span></header>
{{if eq .Tab "/settings"}}<section class="settings"><p class="eyebrow">MAKE YOURSELF AT HOME</p><h1>Model connection</h1><p>Choose a local endpoint or sign in with ChatGPT. Requests and Tool schemas go to your selected provider. New work uses the selected model.</p>
<div class="panel"><h2>Local / OpenAI-compatible</h2><form action="/model/add" method="post"><label>Endpoint URL<input type="url" name="endpoint" value="http://127.0.0.1:8000/v1" required></label><label>Model ID<input name="model" required></label><button>Add connection</button></form></div>
<div class="panel"><h2>OpenAI — Sign in with ChatGPT</h2><form method="post" action="/auth/login"><button>Sign in with ChatGPT ↗</button></form><form method="post" action="/auth/cancel"><button class="secondary">Cancel sign-in</button></form><p>{{.Auth.Status}}</p>{{range .Auth.Events}}<p>{{.Kind}} {{if .URL}}<a href="{{authURL .URL}}" target="_blank" rel="noreferrer">Continue sign-in in your browser ↗</a>{{end}} {{.DeviceCode}} {{range .Choices}}{{.}} {{end}}</p>{{end}}
{{if .Accounts}}<form method="post" action="/model/add"><label>Account<select name="account">{{range .Accounts}}<option value="{{.ConnectionID}}">{{.AccountLabel}} · {{.Status}}</option>{{end}}</select></label><label>Model ID<input name="model" required></label><button>Add account model</button></form>{{range .Accounts}}<p>{{.AccountLabel}} · {{.Status}}</p><form method="post" action="/auth/login"><input type="hidden" name="account" value="{{.ConnectionID}}"><button class="secondary">Reconnect account</button></form><form method="post" action="/auth/logout"><input type="hidden" name="account" value="{{.ConnectionID}}"><button class="secondary">Sign out</button></form>{{end}}{{end}}</div>
<div class="panel"><h2>Notifications</h2><p>Current channel: {{.Preference.Channel}}. Automatic prefers Feishu when bound. Ordinary progress stays on the progress page.</p><form method="post" action="/notification"><input name="version" type="hidden" value="{{.Preference.Version}}"><label>Send future notifications to<select name="target"><option value="auto:">Automatic</option><option value="tui:">Terminal</option>{{range .Bindings}}{{if .Enabled}}<option value="{{.Channel}}:{{.ID}}">{{.Channel}} · {{.Destination}}</option>{{end}}{{end}}</select></label><button>Save notification channel</button></form></div><h2>Saved models</h2>{{range .Profiles}}<div class="panel"><h3>{{.ModelID}}</h3><p>{{.ProviderKind}} · {{.Status}}</p><form method="post" action="/model/test"><input name="profile" type="hidden" value="{{.ProfileID}}"><button class="secondary">Test connection</button></form><form method="post" action="/model/select"><input name="profile" type="hidden" value="{{.ProfileID}}"><input name="profile_version" type="hidden" value="{{.Version}}"><input name="config_version" type="hidden" value="{{$.Status.ConfigVersion}}"><label class="check"><input name="consent" type="checkbox" value="yes" required>Use this provider for new work</label><button>Select model</button></form></div>{{else}}<p>No models connected yet.</p>{{end}}<p><a href="/licenses.txt">Open source notices</a></p></section>
{{else if eq .Tab "/progress"}}<section class="settings"><p class="eyebrow">A LITTLE PEACE OF MIND</p><h1>Your ongoing work</h1><p>Progress updates appear here as work moves forward. Continue, pause or cancel work in conversation.</p>{{range .Activity}}<article class="panel"><h2>{{.Title}}</h2><p class="state">{{.Status}}</p><p>{{.ProgressSummary}}</p></article>{{else}}<div class="empty"><h2>All clear for now.</h2><p>When you hand something over, you can follow it here.</p></div>{{end}}</section>
{{else}}<section class="thread">{{if eq .Status.Readiness "setup_required"}}<div class="panel"><h2>Let's connect your model.</h2><p>Choose how Erii should help you think and get things done.</p><a class="button" href="/settings">Set up Erii →</a></div>{{end}}
{{range .Messages}}<article class="message {{.Role}}"><div class="message-label">{{if eq .Role "user"}}You{{else}}<img class="avatar" src="/brand/eri-icon-192.png" alt="Erii default agent avatar" width="28" height="28">Erii{{end}}</div><div data-ui-spec="{{.Projection.Spec}}" data-ui-fallback="{{.Projection.Fallback}}">{{.Projection.HTML}}</div></article>{{else}}<div class="welcome"><img class="welcome-avatar" src="/brand/eri-icon-192.png" alt="Erii default agent avatar" width="72" height="72"><p class="eyebrow">A LITTLE ROOM TO THINK</p><h1>What's on your mind?</h1><p>Plan something, work through an idea,<br>or hand over the next thing on your list.</p></div>{{end}}
{{range .Approvals}}<div data-ui-spec="{{.Spec}}" data-ui-fallback="{{.Fallback}}">{{.HTML}}</div>{{end}}</section><footer><div id="composer-root" data-source-id="{{.SourceID}}"><form class="composer" action="/message" method="post" enctype="multipart/form-data"><input type="hidden" name="source_id" value="{{.SourceID}}"><label for="message" class="sr-only">Message Erii</label><textarea id="message" name="text" placeholder="Tell Erii what you need…" required rows="2" maxlength="16384"></textarea><label class="attachment-picker">Attach a file<input type="file" name="file" aria-label="Attach a file"></label><div><small>Shared with your TUI and bound Feishu conversation</small><button aria-label="Send message">Send ↑</button></div></form></div><p class="footnote">Erii asks for approval before sensitive external actions.</p></footer>{{end}}</main>{{end}}</body></html>`))
