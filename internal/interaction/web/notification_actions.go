package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/z-chenhao/collie/internal/interaction"
)

func (s *Server) preferNotification(r *http.Request) error {
	version, e := strconv.ParseInt(r.FormValue("version"), 10, 64)
	if e != nil {
		return e
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
