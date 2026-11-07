package web

import (
	"net/url"
	"sort"
	"strings"

	"github.com/z-chenhao/assistant/internal/connectedapps"
	"github.com/z-chenhao/assistant/internal/connector/composio"
)

// Discovery is private presentation policy over the existing editorial catalogue.
// It does not assert project availability; Connect still resolves live metadata.
var featuredApps = []string{"gmail", "googledrive", "googlecalendar", "github", "slack", "notion"}

var discoveryCategories = []composio.Category{
	{ID: "communication", Name: "Communication and calendars"},
	{ID: "storage", Name: "Documents and knowledge"},
	{ID: "productivity", Name: "Tasks and planning"},
	{ID: "development", Name: "Development tools"},
	{ID: "design", Name: "Design and creation"},
	{ID: "business", Name: "Business tools"},
	{ID: "personal", Name: "Personal life"},
}

func discoveryCategory(item composio.Toolkit) string {
	switch item.Slug {
	case "googlecalendar", "calendly":
		return "communication"
	case "notion", "airtable":
		return "storage"
	}
	return item.Meta.Categories[0].ID // Editorial entries always have one category.
}

func sortApps(items []appCard) {
	sort.Slice(items, func(i, j int) bool {
		a, b := strings.ToLower(items[i].Name), strings.ToLower(items[j].Name)
		if a == b {
			return items[i].Slug < items[j].Slug
		}
		return a < b
	})
}

func (s *Server) loadAppDiscovery(state connectedapps.State, collection string) (*appsView, error) {
	v := &appsView{View: connectedapps.View{State: state, Suggested: state.KeyRef == ""}, Managed: s.managedApps(state), Collection: collection}
	bySlug := map[string]appCard{}
	groups := map[string][]appCard{}
	for _, item := range connectedapps.Suggestions("", "") {
		card := s.cardFor(item, state)
		bySlug[item.Slug] = card
		groups[discoveryCategory(item)] = append(groups[discoveryCategory(item)], card)
	}
	if collection != "" {
		for _, category := range discoveryCategories {
			if category.ID == collection {
				v.Items = groups[collection]
				sortApps(v.Items)
				v.Groups = []appGroup{{Title: category.Name, Items: v.Items}}
				return v, nil
			}
		}
		return nil, connectedapps.ErrInvalid
	}
	featured := appGroup{Title: "Featured apps"}
	for _, slug := range featuredApps {
		featured.Items = append(featured.Items, bySlug[slug])
		delete(bySlug, slug)
	}
	v.Groups = append(v.Groups, featured)
	v.Items = append(v.Items, featured.Items...)
	for _, category := range discoveryCategories {
		group := appGroup{Title: category.Name, URL: "/settings/apps?collection=" + url.QueryEscape(category.ID)}
		for _, item := range groups[category.ID] {
			if _, ok := bySlug[item.Slug]; ok {
				group.Items = append(group.Items, item)
			}
		}
		sortApps(group.Items)
		if len(group.Items) > 6 {
			group.Items = group.Items[:6]
		}
		if len(group.Items) > 0 {
			v.Groups = append(v.Groups, group)
			v.Items = append(v.Items, group.Items...)
		}
	}
	return v, nil
}
