package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/z-chenhao/assistant/internal/connectedapps"
	"github.com/z-chenhao/assistant/internal/connector/composio"
)

type AppConnections interface {
	State(context.Context) (connectedapps.State, error)
	Detail(context.Context, string) (composio.Toolkit, error)
	View(context.Context, string, string, string) (connectedapps.View, error)
	ConfigureProject(context.Context, string, int64, map[string]string) error
	Connect(context.Context, string, string, string) (composio.Link, error)
	Continue(context.Context, string) (composio.Link, error)
	Reconnect(context.Context, string) (composio.Link, error)
	Check(context.Context, string) error
	Disconnect(context.Context, string) error
}
type appCard struct {
	composio.Toolkit
	Connected                       bool
	PublicActive                    bool
	Managed                         bool
	Pending                         bool
	Icon, Initial, Description, URL string
}
type appGroup struct {
	Title string
	Items []appCard
}
type appsView struct {
	connectedapps.View
	Items                                    []appCard
	ConnectedOnly                            bool
	ConnectURL, ConnectionID, AuthConfigText string
	ExpiresAt                                int64
	Groups                                   []appGroup
	Managed                                  []appCard
	Selected                                 *appCard
}

// Inventory includes every account even on a native app detail page, so opening
// another app in the dialog never loses its current connection controls.
func (v *appsView) Inventory() page {
	all := *v
	all.Selected = nil
	return page{Apps: &all}
}

func (s *Server) cardFor(item composio.Toolkit, state connectedapps.State) appCard {
	card := appCard{Toolkit: item, URL: "/settings/apps?app=" + item.Slug}
	if card.Name == "" {
		card.Name = card.Slug
	}
	initial := []rune(card.Name)
	if len(initial) > 0 {
		card.Initial = strings.ToUpper(string(initial[0]))
	}
	s.appLogos.register(item.Slug, item.Meta.Logo)
	card.Icon = "/apps/logo/" + item.Slug
	card.Description = connectedapps.Description(item.Slug)
	if card.Description == "" {
		card.Description = item.Meta.Description
	}
	if card.Description == "" {
		card.Description = "Bring this app into your everyday work"
	}
	caption := []rune(card.Description)
	if len(caption) > 150 {
		card.Description = string(caption[:147]) + "…"
	}
	for _, c := range state.Connections {
		if c.Toolkit != item.Slug {
			continue
		}
		if c.Status != "disconnected" || !c.RevocationConfirmed {
			card.Managed = true
		}
		if c.Status == "active" {
			card.Connected = true
			card.PublicActive = c.Public
		}
		if c.Status == "pending" {
			card.Pending = true
		}
	}
	return card
}
func (s *Server) managedApps(state connectedapps.State) []appCard {
	var result []appCard
	seen := map[string]bool{}
	for _, c := range state.Connections {
		if seen[c.Toolkit] || c.Status == "disconnected" && c.RevocationConfirmed {
			continue
		}
		seen[c.Toolkit] = true
		result = append(result, s.cardFor(composio.Toolkit{Slug: c.Toolkit, Name: c.Name}, state))
	}
	sort.Slice(result, func(i, j int) bool { return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name) })
	return result
}
func groupApps(items []appCard, filtered bool) []appGroup {
	if len(items) == 0 {
		return nil
	}
	if filtered {
		return []appGroup{{Title: "Apps", Items: items}}
	}
	order := []string{"Communication", "Productivity", "Documents and storage", "Development", "Design", "Business", "Personal life", "More apps"}
	groups := map[string][]appCard{}
	for _, item := range items {
		name := "More apps"
		if len(item.Meta.Categories) > 0 {
			name = item.Meta.Categories[0].Name
			if name == "" {
				name = "More apps"
			}
		}
		if _, ok := groups[name]; !ok {
			known := false
			for _, category := range order {
				if category == name {
					known = true
				}
			}
			if !known {
				order = append(order, name)
			}
		}
		groups[name] = append(groups[name], item)
	}
	var result []appGroup
	for _, name := range order {
		if len(groups[name]) > 0 {
			result = append(result, appGroup{Title: name, Items: groups[name]})
		}
	}
	return result
}

func (s *Server) loadAppService(ctx context.Context, p *page) error {
	if s.Apps == nil {
		return connectedapps.ErrUnavailable
	}
	state, err := s.Apps.State(ctx)
	if err != nil {
		return err
	}
	text := "{}"
	if len(state.AuthConfigs) > 0 {
		raw, _ := json.MarshalIndent(state.AuthConfigs, "", "  ")
		text = string(raw)
	}
	p.Settings = &settingsView{Section: "resources"}
	p.Apps = &appsView{View: connectedapps.View{State: state}, AuthConfigText: text}
	return nil
}
func (s *Server) loadApps(ctx context.Context, r *http.Request, p *page) error {
	if s.Apps == nil {
		return connectedapps.ErrUnavailable
	}
	p.Settings = &settingsView{Section: "apps"}
	if slug := r.URL.Query().Get("app"); slug != "" {
		state, err := s.Apps.State(ctx)
		if err != nil {
			return err
		}
		item, err := s.Apps.Detail(ctx, slug)
		if err != nil {
			return err
		}
		card := s.cardFor(item, state)
		p.Apps = &appsView{View: connectedapps.View{State: state, Suggested: state.KeyRef == ""}, Selected: &card, Managed: s.managedApps(state)}
		return nil
	}
	if r.URL.Query().Get("view") == "connected" {
		state, err := s.Apps.State(ctx)
		p.Apps = &appsView{View: connectedapps.View{State: state, Suggested: state.KeyRef == ""}, ConnectedOnly: true, Managed: s.managedApps(state)}
		p.Apps.Items = p.Apps.Managed
		p.Apps.Groups = groupApps(p.Apps.Items, true)
		return err
	}
	v, err := s.Apps.View(ctx, r.URL.Query().Get("search"), r.URL.Query().Get("category"), r.URL.Query().Get("cursor"))
	p.Apps = &appsView{View: v, Managed: s.managedApps(v.State)}
	for _, item := range v.Catalog.Items {
		card := s.cardFor(item, v.State)
		p.Apps.Items = append(p.Apps.Items, card)
	}
	p.Apps.Groups = groupApps(p.Apps.Items, v.Search != "" || v.Category != "" || r.URL.Query().Get("cursor") != "")
	return err
}

func (s *Server) appDetail(w http.ResponseWriter, r *http.Request) {
	p := page{}
	if r.URL.Query().Get("app") == "" {
		fail(w, 400)
		return
	}
	if err := s.loadApps(r.Context(), r, &p); err != nil {
		s.pageError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = localizedLayout(requestLanguage(r)).ExecuteTemplate(w, "app-detail", p)
}
func appError(err error) string {
	switch {
	case errors.Is(err, connectedapps.ErrConflict):
		return "App connections changed. Refresh and try again."
	case errors.Is(err, connectedapps.ErrInvalid):
		return "Check the app, account alias and credential reference."
	case errors.Is(err, connectedapps.ErrDenied):
		return "This account cannot be used. Reconnect it to continue."
	case errors.Is(err, connectedapps.ErrAliasRequired):
		return "Choose a label for the additional account."
	case errors.Is(err, connectedapps.ErrLinkExpired):
		return "This authorization link has expired. Reconnect the account to continue."
	default:
		return "App service unavailable. Ask the installation owner to check the connection service."
	}
}
func (s *Server) appServiceAction(w http.ResponseWriter, r *http.Request) error {
	if s.Apps == nil {
		return connectedapps.ErrUnavailable
	}
	revision, err := strconv.ParseInt(r.FormValue("revision"), 10, 64)
	configs := map[string]string{}
	text := r.FormValue("auth_configs")
	if len(text) > 16384 {
		err = connectedapps.ErrInvalid
	} else if text != "" && (json.Unmarshal([]byte(text), &configs) != nil || configs == nil) {
		err = connectedapps.ErrInvalid
	}
	if err == nil {
		err = s.Apps.ConfigureProject(r.Context(), r.FormValue("key_ref"), revision, configs)
	}
	p := page{Tab: "/settings", View: "app-service", Title: "App connection service", Path: "/settings/services/apps"}
	if e := s.loadAppService(r.Context(), &p); e != nil {
		return e
	}
	if err != nil {
		p.Error = appError(err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnprocessableEntity)
	} else {
		p.Notice = "App connection service saved."
	}
	s.render(w, r, p)
	return nil
}
func (s *Server) appAction(w http.ResponseWriter, r *http.Request) error {
	if s.Apps == nil {
		return connectedapps.ErrUnavailable
	}
	var err error
	var link composio.Link
	switch r.FormValue("action") {
	case "connect":
		link, err = s.Apps.Connect(r.Context(), r.FormValue("toolkit"), r.FormValue("alias"), "")
	case "continue":
		link, err = s.Apps.Continue(r.Context(), r.FormValue("connection"))
	case "reconnect":
		link, err = s.Apps.Reconnect(r.Context(), r.FormValue("connection"))
	case "check":
		err = s.Apps.Check(r.Context(), r.FormValue("connection"))
	case "disconnect":
		err = s.Apps.Disconnect(r.Context(), r.FormValue("connection"))
	default:
		err = connectedapps.ErrInvalid
	}
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Header.Get("Accept") == "application/json" {
		if err != nil {
			code := "unavailable"
			if errors.Is(err, connectedapps.ErrAliasRequired) {
				code = "alias_required"
			}
			if errors.Is(err, connectedapps.ErrLinkExpired) {
				code = "authorization_expired"
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			jsonResult(w, map[string]string{"error": translate(requestLanguage(r), appError(err)), "code": code})
			return nil
		}
		state, e := s.Apps.State(r.Context())
		if e != nil {
			return e
		}
		view := page{Apps: &appsView{View: connectedapps.View{State: state}, ConnectURL: link.URL, ConnectionID: link.ConnectionID, ExpiresAt: link.ExpiresAt.UnixMilli(), Managed: s.managedApps(state)}}
		var html bytes.Buffer
		if e = localizedLayout(requestLanguage(r)).ExecuteTemplate(&html, "app-accounts", view); e != nil {
			return e
		}
		var sidebar bytes.Buffer
		if e = localizedLayout(requestLanguage(r)).ExecuteTemplate(&sidebar, "app-sidebar", view); e != nil {
			return e
		}
		jsonResult(w, map[string]string{"accounts_html": html.String(), "sidebar_html": sidebar.String(), "authorization_url": link.URL, "connection_id": link.ConnectionID, "authorization_expires": strconv.FormatInt(link.ExpiresAt.UnixMilli(), 10), "revision": strconv.FormatInt(state.Revision, 10)})
		return nil
	}
	p := page{Tab: "/settings", View: "apps", Title: "Apps", Path: "/settings/apps"}
	next := r.Clone(r.Context())
	u := *r.URL
	u.Path = "/settings/apps"
	q := r.URL.Query()
	q.Del("app")
	if slug := r.FormValue("toolkit"); slug != "" {
		q.Set("app", slug)
	} else {
		state, _ := s.Apps.State(r.Context())
		for _, c := range state.Connections {
			if c.ID == r.FormValue("connection") {
				q.Set("app", c.Toolkit)
				break
			}
		}
	}
	u.RawQuery = q.Encode()
	next.URL = &u
	if e := s.loadApps(r.Context(), next, &p); e != nil {
		return e
	}
	p.Apps.ConnectURL, p.Apps.ConnectionID = link.URL, link.ConnectionID
	p.Apps.ExpiresAt = link.ExpiresAt.UnixMilli()
	if err != nil {
		p.Error = appError(err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnprocessableEntity)
	} else {
		p.Notice = "App settings updated."
	}
	s.render(w, r, p)
	return nil
}
