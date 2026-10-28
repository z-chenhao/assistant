package web

import (
	"context"
	"github.com/z-chenhao/erii/internal/interaction"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/task"
	"net/http/httptest"
	"testing"
)

// Unrelated query methods intentionally remain nil: these pages must not
// depend on history, approvals or another page's read models.
type settingsCore struct{ Core }

func (settingsCore) WebStatus(context.Context) (Status, error)                      { return Status{Status: "idle"}, nil }
func (settingsCore) ChannelBindings(context.Context) ([]interaction.Binding, error) { return nil, nil }
func (settingsCore) ChannelPreference(context.Context) (interaction.Preference, error) {
	return interaction.Preference{}, nil
}

type settingsModels struct{ Models }

func (settingsModels) ListProfiles(context.Context) ([]llm.Profile, error) { return nil, nil }

type progressCore struct{ Core }

func (progressCore) WebStatus(context.Context) (Status, error)        { return Status{Status: "idle"}, nil }
func (progressCore) Activity(context.Context) ([]task.Summary, error) { return nil, nil }
func TestSettingsAndProgressDoNotQueryConversation(t *testing.T) {
	for _, path := range []string{"/settings", "/progress"} {
		t.Run(path, func(t *testing.T) {
			server := &Server{Core: settingsCore{}, Models: settingsModels{}}
			if path == "/progress" {
				server.Core = progressCore{}
				server.Models = nil
			}
			response := httptest.NewRecorder()
			server.show(response, httptest.NewRequest("GET", path, nil))
			if response.Code != 200 {
				t.Fatal(response.Code, response.Body.String())
			}
		})
	}
}
