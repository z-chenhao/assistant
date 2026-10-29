package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

// CapabilitySettings is an owner control plane. It never enters model tools or
// mutates the execution registry and frozen capability snapshots.
type CapabilitySettings interface {
	List(context.Context) ([]Capability, error)
	Detail(context.Context, string) (Capability, error)
	Test(context.Context, string, string, string) (Capability, error)
	Save(context.Context, string, string, string) error
}
type Capability struct {
	ID, Title, Description, State, Revision, Text, Example, Help, Message, Path string
	Configured, Active, PendingRestart, Verified, Candidate                     bool
	TestedAt                                                                    time.Time
}

var ErrCapabilityConflict = errors.New("capability configuration changed; reopen the page and test again")
var ErrCapabilityInvalid = errors.New("invalid capability configuration; check the documented fields and separate private paths")
var ErrCapabilityUnverified = errors.New("test this exact configuration successfully before saving")
var ErrCapabilityMigration = errors.New("changing an existing memory key or embedding index identity requires a separate data migration; keep the current identity here")
var ErrCapabilityUnavailable = errors.New("capability settings unavailable")

func capabilityURL(id string) string { return "/settings/capabilities/" + id }
func (s *Server) loadCapabilities(ctx context.Context, r *http.Request, p *page) error {
	if s.Capabilities == nil {
		return ErrCapabilityUnavailable
	}
	p.Settings = &settingsView{Section: "capabilities"}
	var err error
	if r.URL.Path == "/settings/capabilities" {
		p.Capabilities, err = s.Capabilities.List(ctx)
		return err
	}
	id := strings.TrimPrefix(r.URL.Path, "/settings/capabilities/")
	v, err := s.Capabilities.Detail(ctx, id)
	p.Capability = &v
	return err
}
func (s *Server) capabilityAction(w http.ResponseWriter, r *http.Request) error {
	if len(r.FormValue("configuration")) > 65536 {
		http.Error(w, "configuration too large", http.StatusRequestEntityTooLarge)
		return nil
	}
	id := r.FormValue("capability")
	if !validCapability(id) || s.Capabilities == nil {
		return ErrCapabilityInvalid
	}
	next := r.Clone(r.Context())
	u := *r.URL
	u.Path = capabilityURL(id)
	u.RawQuery = ""
	next.URL = &u
	p, err := s.page(r.Context(), next)
	if err != nil {
		return err
	}
	switch r.FormValue("action") {
	case "test":
		_, e := s.Capabilities.Test(r.Context(), id, r.FormValue("revision"), r.FormValue("configuration"))
		err = e
		if e == nil {
			http.Redirect(w, r, capabilityURL(id), 303)
			return nil
		}
	case "save":
		err = s.Capabilities.Save(r.Context(), id, r.FormValue("revision"), r.FormValue("configuration"))
		if err == nil {
			http.Redirect(w, r, capabilityURL(id), 303)
			return nil
		}
	default:
		err = ErrCapabilityInvalid
	}
	if err != nil {
		p.Capability.Verified = false
		p.Capability.State = "unavailable"
		p.Capability.Message = ""
		p.Capability.TestedAt = time.Time{}
		p.Capability.Candidate = true
		p.Capability.Text = r.FormValue("configuration")
		p.Error = ErrCapabilityUnavailable.Error()
		for _, known := range []error{ErrCapabilityConflict, ErrCapabilityInvalid, ErrCapabilityUnverified, ErrCapabilityMigration} {
			if errors.Is(err, known) {
				p.Error = known.Error()
			}
		}
		w.WriteHeader(http.StatusConflict)
	}
	s.render(w, p)
	return nil
}
func validCapability(id string) bool {
	switch id {
	case "memory", "embedding", "mcp", "skills", "execution", "computer":
		return true
	}
	return false
}
