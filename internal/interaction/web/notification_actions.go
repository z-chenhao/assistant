package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/interaction"
	"github.com/z-chenhao/assistant/internal/llm"
)

func (s *Server) preferNotification(r *http.Request) error {
	version, e := strconv.ParseInt(r.FormValue("version"), 10, 64)
	if e != nil || version < 0 {
		return interaction.ErrUnauthorized
	}
	channel, binding, ok := strings.Cut(r.FormValue("target"), ":")
	if !ok {
		return interaction.ErrUnauthorized
	}
	if e = s.Core.PreferChannel(r.Context(), channel, binding, version); e != nil {
		return e
	}

	return nil
}

// JSON acknowledgements expose only the confirmed preference revision. Native
// forms retain their redirect; an uncertain write must be checked with a GET.
func (s *Server) notificationAction(w http.ResponseWriter, r *http.Request) error {
	err := s.preferNotification(r)
	if r.Header.Get("Accept") != "application/json" {
		if err == nil {
			http.Redirect(w, r, "/settings/notifications?notice=saved", http.StatusSeeOther)
		}
		return err
	}
	if err != nil {
		code := http.StatusServiceUnavailable
		if errors.Is(err, interaction.ErrUnauthorized) {
			code = http.StatusForbidden
		}
		if errors.Is(err, llm.ErrConflict) || errors.Is(err, agent.ErrConflict) {
			code = http.StatusConflict
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Notification preference could not be saved."})
		return nil
	}
	preference, err := s.Core.ChannelPreference(r.Context())
	if err != nil {
		fail(w, http.StatusServiceUnavailable)
		return nil
	}
	target := preference.Channel + ":"
	if preference.BindingID != nil {
		target += *preference.BindingID
	}
	jsonResult(w, struct {
		Target  string `json:"target"`
		Version int64  `json:"version"`
	}{target, preference.Version})
	return nil
}
