package composio

import (
	"context"
	"errors"
	"net/url"
)

// Only metadata is decoded. Auth config responses can also contain private
// client credentials; those fields must never leave this adapter.
type authConfig struct {
	ID      string `json:"id"`
	Managed bool   `json:"is_composio_managed"`
	Status  string `json:"status"`
	Router  bool   `json:"is_enabled_for_tool_router"`
	Created string `json:"created_at"`
	Toolkit struct {
		Slug string `json:"slug"`
	} `json:"toolkit"`
}

func (c Client) AuthConfig(ctx context.Context, toolkit, override string) (string, error) {
	if !ValidID(toolkit) || override != "" && !ValidID(override) {
		return "", &Error{Status: 400}
	}
	if override != "" {
		var config authConfig
		if err := c.request(ctx, "GET", "/auth_configs/"+override, nil, nil, &config); err != nil {
			return "", err
		}
		if config.ID != override || config.Toolkit.Slug != toolkit || config.Status != "ENABLED" || !config.Router {
			return "", &Error{Status: 400}
		}
		return override, nil
	}
	var chosen authConfig
	cursor := ""
	seen := map[string]bool{}
	for range 20 {
		var page struct {
			Items []authConfig `json:"items"`
			Next  string       `json:"next_cursor"`
		}
		err := c.request(ctx, "GET", "/auth_configs", url.Values{"toolkit_slug": {toolkit}, "is_composio_managed": {"false"}, "show_disabled": {"false"}, "limit": {"200"}, "cursor": {cursor}}, nil, &page)
		if err != nil {
			// Scoped keys without auth-config read permission can still use
			// Composio-managed sign-in. The link endpoint decides availability.
			var failure *Error
			if errors.As(err, &failure) && (failure.Status == 401 || failure.Status == 403) {
				return "", nil
			}
			return "", err
		}
		for _, config := range page.Items {
			if config.Toolkit.Slug == toolkit && !config.Managed && config.Status == "ENABLED" && config.Router && ValidID(config.ID) {
				if chosen.ID == "" || config.Created > chosen.Created || config.Created == chosen.Created && config.ID > chosen.ID {
					chosen = config
				}
			}
		}
		if page.Next == "" {
			return chosen.ID, nil
		}
		if seen[page.Next] {
			return "", &Error{}
		}
		seen[page.Next] = true
		cursor = page.Next
	}
	return "", &Error{}
}
