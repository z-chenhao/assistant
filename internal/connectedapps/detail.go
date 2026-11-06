package connectedapps

import (
	"context"

	"github.com/z-chenhao/assistant/internal/connector/composio"
)

// Detail remains readable when catalogue metadata is unavailable. Local
// names/descriptions do not authorize a connection or bypass dispatch checks.
func (s *Service) Detail(ctx context.Context, slug string) (composio.Toolkit, error) {
	if !composio.ValidID(slug) {
		return composio.Toolkit{}, ErrInvalid
	}
	state, err := s.State(ctx)
	if err != nil {
		return composio.Toolkit{}, err
	}
	var fallback composio.Toolkit
	for _, app := range Suggestions("", "") {
		if app.Slug == slug {
			fallback = app
			break
		}
	}
	for _, c := range state.Connections {
		if c.Toolkit == slug && fallback.Slug == "" {
			fallback.Slug = slug
			fallback.Name = c.Name
			fallback.NoAuth = c.Public
		}
	}
	if state.KeyRef != "" {
		app, e := s.client(state).Toolkit(ctx, slug)
		if e == nil && app.Slug == slug {
			return app, nil
		}
		if fallback.Slug == "" {
			return composio.Toolkit{}, ErrUnavailable
		}
	}
	if fallback.Slug == "" {
		return fallback, ErrInvalid
	}
	return fallback, nil
}
