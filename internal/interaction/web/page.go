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
	Role     string
	Sequence int64
	HTML     template.HTML
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
	Approvals  []template.HTML
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
			p.Approvals = append(p.Approvals, ui.HTML(raw, spec.Fallback))
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
		p.Messages = append(p.Messages, shownMessage{m.Role, m.Sequence, ui.HTML(raw, fallback)})
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

var layout = template.Must(template.New("page").Funcs(template.FuncMap{"authURL": AuthURL}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Erii · Personal assistant</title><link rel="stylesheet" href="/style.css"><script defer src="/app.js"></script></head><body>
{{if .Login}}<main class="login"><div class="mark">e.</div><p class="eyebrow">YOUR PERSONAL ASSISTANT</p><h1>Welcome to Erii</h1><p>Connect to your private workspace.</p><form action="/session" method="post"><label>Workspace access token<input type="password" name="token" required autocomplete="off"></label><button>Open workspace →</button></form><small>The installation owner configures access through the credential store.</small></main>
{{else}}<aside><a class="brand" href="/">e.<span>Erii</span></a><p class="eyebrow">WORKSPACE</p><nav><a href="/" {{if eq .Tab "/"}}aria-current="page"{{end}}>◌ &nbsp; Conversation</a><a href="/progress" {{if eq .Tab "/progress"}}aria-current="page"{{end}}>◷ &nbsp; Ongoing work</a><a href="/settings" {{if eq .Tab "/settings"}}aria-current="page"{{end}}>⚙ &nbsp; Settings</a></nav><div class="aside-bottom"><span class="dot"></span> Private workspace<br><small>One conversation, wherever you are.</small></div></aside>
<main><header><span>{{if eq .Tab "/progress"}}Ongoing work{{else if eq .Tab "/settings"}}Settings{{else}}Conversation{{end}}</span><span class="state"><span class="dot"></span> {{.Status.Status}}</span></header>
{{if eq .Tab "/settings"}}<section class="settings"><p class="eyebrow">MAKE YOURSELF AT HOME</p><h1>Model connection</h1><p>Choose a local endpoint or sign in with ChatGPT. Requests and Tool schemas go to your selected provider. New work uses the selected model.</p>
<div class="panel"><h2>Local / OpenAI-compatible</h2><form action="/model/add" method="post"><label>Endpoint URL<input type="url" name="endpoint" value="http://127.0.0.1:8000/v1" required></label><label>Model ID<input name="model" required></label><button>Add connection</button></form></div>
<div class="panel"><h2>OpenAI — Sign in with ChatGPT</h2><form method="post" action="/auth/login"><button>Sign in with ChatGPT ↗</button></form><form method="post" action="/auth/cancel"><button class="secondary">Cancel sign-in</button></form><p>{{.Auth.Status}}</p>{{range .Auth.Events}}<p>{{.Kind}} {{if .URL}}<a href="{{authURL .URL}}" target="_blank" rel="noreferrer">Continue sign-in in your browser ↗</a>{{end}} {{.DeviceCode}} {{range .Choices}}{{.}} {{end}}</p>{{end}}
{{if .Accounts}}<form method="post" action="/model/add"><label>Account<select name="account">{{range .Accounts}}<option value="{{.ConnectionID}}">{{.AccountLabel}} · {{.Status}}</option>{{end}}</select></label><label>Model ID<input name="model" required></label><button>Add account model</button></form>{{range .Accounts}}<p>{{.AccountLabel}} · {{.Status}}</p><form method="post" action="/auth/login"><input type="hidden" name="account" value="{{.ConnectionID}}"><button class="secondary">Reconnect account</button></form><form method="post" action="/auth/logout"><input type="hidden" name="account" value="{{.ConnectionID}}"><button class="secondary">Sign out</button></form>{{end}}{{end}}</div>
<div class="panel"><h2>Notifications</h2><p>Current channel: {{.Preference.Channel}}. Automatic prefers Feishu when bound. Ordinary progress stays on the progress page.</p><form method="post" action="/notification"><input name="version" type="hidden" value="{{.Preference.Version}}"><label>Send future notifications to<select name="target"><option value="auto:">Automatic</option><option value="tui:">Terminal</option>{{range .Bindings}}{{if .Enabled}}<option value="{{.Channel}}:{{.ID}}">{{.Channel}} · {{.Destination}}</option>{{end}}{{end}}</select></label><button>Save notification channel</button></form></div><h2>Saved models</h2>{{range .Profiles}}<div class="panel"><h3>{{.ModelID}}</h3><p>{{.ProviderKind}} · {{.Status}}</p><form method="post" action="/model/test"><input name="profile" type="hidden" value="{{.ProfileID}}"><button class="secondary">Test connection</button></form><form method="post" action="/model/select"><input name="profile" type="hidden" value="{{.ProfileID}}"><input name="profile_version" type="hidden" value="{{.Version}}"><input name="config_version" type="hidden" value="{{$.Status.ConfigVersion}}"><label class="check"><input name="consent" type="checkbox" value="yes" required>Use this provider for new work</label><button>Select model</button></form></div>{{else}}<p>No models connected yet.</p>{{end}}</section>
{{else if eq .Tab "/progress"}}<section class="settings"><p class="eyebrow">A LITTLE PEACE OF MIND</p><h1>Your ongoing work</h1><p>Progress updates appear here as work moves forward. Continue, pause or cancel work in conversation.</p>{{range .Activity}}<article class="panel"><h2>{{.Title}}</h2><p class="state">{{.Status}}</p><p>{{.ProgressSummary}}</p></article>{{else}}<div class="empty"><h2>All clear for now.</h2><p>When you hand something over, you can follow it here.</p></div>{{end}}</section>
{{else}}<section class="thread">{{if eq .Status.Readiness "setup_required"}}<div class="panel"><h2>Let's connect your model.</h2><p>Choose how Erii should help you think and get things done.</p><a class="button" href="/settings">Set up Erii →</a></div>{{end}}
{{range .Messages}}<article class="message {{.Role}}"><div class="message-label">{{if eq .Role "user"}}YOU{{else}}ERII{{end}}<span>#{{.Sequence}}</span></div>{{.HTML}}</article>{{else}}<div class="welcome"><p class="eyebrow">A CLEARER DAY STARTS HERE</p><h1>What's on your mind?</h1><p>Plan something, work through an idea,<br>or hand over the next thing on your list.</p></div>{{end}}
{{range .Approvals}}{{.}}{{end}}</section><footer><form class="composer" action="/message" method="post" enctype="multipart/form-data"><input type="hidden" name="source_id" value="{{.SourceID}}"><label for="message" class="sr-only">Message Erii</label><textarea id="message" name="text" placeholder="Tell Erii what you need…" required rows="2" maxlength="16384"></textarea><label class="attachment-picker">Attach a file<input type="file" name="file" aria-label="Attach a file"></label><div><small>Shared with your TUI and bound Feishu conversation</small><button aria-label="Send message">Send ↑</button></div></form><p class="footnote">Erii asks for approval before sensitive external actions.</p></footer>{{end}}</main>{{end}}</body></html>`))

const script = `let changed=false;document.addEventListener('input',()=>changed=true);setInterval(()=>{if(!changed&&document.visibilityState==='visible'&&!document.activeElement.matches('input,textarea,button'))location.reload()},5000);`
const style = `:root{font-family:Inter,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:#24352f;background:#fbfcf9;font-synthesis:none}*{box-sizing:border-box}body{margin:0}a{color:#306c55}aside{width:238px;position:fixed;inset:0 auto 0 0;padding:32px 24px;background:#f0f3ec;border-right:1px solid #e0e6dc}.brand{display:flex;align-items:center;gap:14px;text-decoration:none;font-family:Georgia,serif;font-size:40px;font-weight:bold}.brand span{font:600 23px Inter,sans-serif}.eyebrow{font-size:10px;letter-spacing:.18em;color:#728077;font-weight:600}aside>.eyebrow{margin:55px 12px 15px}nav{display:grid;gap:8px}nav a{font-size:14px;padding:13px 12px;border-radius:8px;text-decoration:none;color:#607066}nav a[aria-current]{background:#e1e8da;color:#253e31;font-weight:600}.aside-bottom{position:absolute;bottom:32px;font-size:12px;line-height:2}.aside-bottom small{font-size:10px;color:#839084}.dot{display:inline-block;width:7px;height:7px;border-radius:50%;background:#568567;margin-right:6px}main{margin-left:238px;min-height:100vh}header{height:81px;border-bottom:1px solid #e5e9e1;display:flex;align-items:center;justify-content:space-between;padding:0 42px;font-size:14px}.state{color:#75816f;font-size:12px}h1{font-family:Georgia,serif;font-size:39px;line-height:1.2;font-weight:400;letter-spacing:-.02em}h2{font-size:18px;font-weight:500}h3{font-size:16px}.thread{max-width:830px;padding:40px 36px 220px;margin:auto}.welcome{padding-top:19vh;text-align:center}.welcome h1{margin:18px 0}.welcome>p:last-child,.settings>p{color:#7a8478;line-height:1.8;font-size:14px}.message{padding:23px 0;line-height:1.7;font-size:15px}.message.user{border-bottom:1px solid #e8ebe3}.message-label{font-size:10px;letter-spacing:.12em;color:#83917e;margin-bottom:12px}.message-label span{float:right;font-size:9px}.semantic{white-space:pre-wrap;overflow-wrap:anywhere}.artifact{display:block;padding:14px;border:1px solid #e1e7dd;border-radius:8px;margin-top:12px;font-size:13px}.approval{border:1px solid #d9dfcb;background:#f4f5e9;padding:20px;border-radius:12px}.settings{max-width:800px;padding:50px 40px;margin:auto}.settings h1{margin-bottom:16px}.panel{background:white;border:1px solid #e1e6da;border-radius:12px;padding:24px;margin:22px 0}.panel form+form{margin-top:15px}.panel p{line-height:1.6;color:#727e6c;font-size:13px}label{display:block;font-size:12px;margin-bottom:14px;color:#63705c}input,textarea,select{font:inherit;width:100%;border:1px solid #dce2d4;border-radius:7px;padding:11px;background:#fdfefa;color:#2c3b29;margin-top:7px;outline-color:#a1b294}button,.button{font:500 12px Inter,sans-serif;background:#3c5d46;color:#fff;border:0;border-radius:7px;padding:12px 19px;cursor:pointer;text-decoration:none;display:inline-block}.secondary{background:#eef2e8;color:#4c6548;margin-left:6px}.check{display:flex;align-items:center;gap:8px}.check input{width:auto}.empty{padding:65px 0}.empty p{font-size:14px;color:#83917e}footer{position:fixed;bottom:0;left:238px;right:0;background:linear-gradient(transparent,#fbfcf9 20%);padding:28px 36px 16px}.composer{max-width:758px;margin:auto;border:1px solid #dce3d3;background:white;box-shadow:0 3px 16px #263d2010;border-radius:15px;padding:12px 17px}.composer textarea{border:0;resize:vertical;margin:0;padding:5px;background:white;font-size:14px}.attachment-picker{font-size:10px;margin:5px 0;color:#87917d}.attachment-picker input{width:auto;font-size:10px;border:0;padding:2px}.composer>div{display:flex;justify-content:space-between;align-items:center}.composer small,.footnote{font-size:10px;color:#97a08f}.composer button{padding:9px 14px}.footnote{text-align:center;margin:12px 0 0}.login{margin:10vh auto;padding:40px;max-width:460px;min-height:0}.login form{margin:30px 0}.login>p{color:#7a8478;font-size:14px}.login small{font-size:11px;color:#87917d}.mark{font:bold 54px Georgia;color:#3c5d46}.sr-only{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0,0,0,0)}table{border-collapse:collapse;font-size:13px}td,th{border:1px solid #dae2d2;padding:8px}@media(max-width:700px){aside{position:static;width:auto;padding:15px}.brand{font-size:28px}.brand span{font-size:18px}aside>.eyebrow,.aside-bottom{display:none}nav{display:flex;margin-top:15px;gap:3px}nav a{font-size:11px;padding:9px}main{margin:0}header{height:50px;padding:0 24px}.thread{padding:24px 20px 210px}.settings{padding:25px 20px}footer{left:0;padding:20px 15px 12px}.composer small{max-width:180px;font-size:9px}h1{font-size:30px}.welcome{padding-top:12vh}}`
