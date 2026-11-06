package web

import (
	"context"
	"errors"
	"io"
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
	Automatic                                                                   bool
	ParentMemory                                                                bool
	Checked                                                                     bool
	DefaultKeyRef                                                               string
	ID, Title, Description, State, Revision, Text, Example, Help, Message, Path string
	Configured, Active, PendingRestart, Verified, Candidate                     bool
	TestedAt                                                                    time.Time
	Form                                                                        *capabilityForm
	Advanced                                                                    bool
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
		visible := p.Capabilities[:0]
		for _, v := range p.Capabilities {
			if v.ID != "embedding" {
				visible = append(visible, v)
			}
		}
		p.Capabilities = visible
		return err
	}
	id := strings.TrimPrefix(r.URL.Path, "/settings/capabilities/")
	var v Capability
	if id != "mcp" && (r.URL.Query().Get("checked") == "1" || r.URL.Query().Get("editor") == "advanced") {
		// Retain checked-candidate navigation for existing native clients.
		v, err = s.Capabilities.Detail(ctx, id)
	} else {
		// Ordinary forms edit saved settings, never a previous failed probe.
		// List projects the current document; Detail can project a test draft.
		var items []Capability
		items, err = s.Capabilities.List(ctx)
		found := false
		for _, item := range items {
			if item.ID == id {
				v, found = item, true
				break
			}
		}
		if err == nil && !found {
			err = ErrCapabilityInvalid
		}
	}
	v.ParentMemory = id == "embedding"
	v.Checked = r.URL.Query().Get("checked") == "1"
	prepareCapabilityForm(&v)
	v.Advanced = r.URL.Query().Get("editor") == "advanced"
	p.Capability = &v
	return err
}
func (s *Server) capabilityAction(w http.ResponseWriter, r *http.Request) error {
	if r.MultipartForm != nil && len(r.MultipartForm.File["configuration_file"]) > 0 {
		file, _, err := r.FormFile("configuration_file")
		if err != nil {
			return ErrCapabilityInvalid
		}
		defer file.Close()
		body, err := io.ReadAll(io.LimitReader(file, 65537))
		if err != nil || len(body) > 65536 {
			http.Error(w, "configuration too large", http.StatusRequestEntityTooLarge)
			return nil
		}
		r.PostForm.Set("configuration", string(body))
		r.Form.Set("configuration", string(body))
	}
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
	text := r.FormValue("configuration")
	var guided *capabilityForm
	if r.FormValue("editor") == "guided" {
		// The persisted document is an implementation detail, never round-tripped
		// through the user-facing form. Test/Save still enforce its revision.
		text = p.Capability.Text
		guided, err = postedCapabilityForm(id, r.PostForm)
		if err == nil && id == "mcp" && editCapabilityRows(guided, r.FormValue("action")) {
			if strings.HasPrefix(r.FormValue("action"), "remove-") {
				// Removal is a real edit. Persist a complete remaining form rather
				// than reporting an unsaved row deletion as the new baseline.
				if _, e := guidedCapabilityText(id, text, guided); e == nil {
					r.PostForm.Set("action", "apply")
					r.Form.Set("action", "apply")
				} else {
					guided.Error = e.Error()
				}
			}
			if r.FormValue("action") != "apply" {
				p.Capability.Form = guided
				p.Capability.Revision = r.FormValue("revision")
				p.Capability.Verified = false
				p.Capability.Candidate = true
				p.Capability.Text = text
				p.Capability.Message = ""
				p.Capability.TestedAt = time.Time{}
				p.Capability.State = "configured"
				s.render(w, r, p)
				return nil
			}
		}
		if err == nil {
			text, err = guidedCapabilityText(id, text, guided)
		}
	}
	status := http.StatusConflict
	var probeMessage string
	location := capabilityURL(id)
	if guided == nil {
		location += "?editor=advanced"
	}
	if err == nil {
		switch r.FormValue("action") {
		case "apply":
			var checked Capability
			checked, err = s.Capabilities.Test(r.Context(), id, r.FormValue("revision"), text)
			if err == nil && !checked.Verified {
				probeMessage = checked.Message
				err = ErrCapabilityUnavailable
				status = http.StatusUnprocessableEntity
			}
			if err == nil {
				err = s.Capabilities.Save(r.Context(), id, r.FormValue("revision"), text)
			}
			if err == nil {
				http.Redirect(w, r, location, http.StatusSeeOther)
				return nil
			}
		case "test":
			_, e := s.Capabilities.Test(r.Context(), id, r.FormValue("revision"), text)
			err = e
			if e == nil {
				if guided != nil {
					location += "?checked=1"
				}
				// A completed probe can be unavailable with no Go error. Detail
				// reports that stored result and only a ready proof enables Save;
				// redirecting does not assert that the connection succeeded.
				http.Redirect(w, r, location, http.StatusSeeOther)
				return nil
			}
		case "save":
			err = s.Capabilities.Save(r.Context(), id, r.FormValue("revision"), text)
			if err == nil {
				http.Redirect(w, r, location, http.StatusSeeOther)
				return nil
			}
		default:
			err = ErrCapabilityInvalid
		}
	} else {
		status = http.StatusUnprocessableEntity
	}
	if err != nil {
		p.Capability.Verified = false
		p.Capability.State = "unavailable"
		p.Capability.Message = ""
		if probeMessage != "" {
			p.Capability.Message = probeMessage
		}
		p.Capability.TestedAt = time.Time{}
		p.Capability.Candidate = true
		p.Capability.Text = text
		prepareCapabilityForm(p.Capability)
		p.Capability.Advanced = guided == nil
		if guided != nil {
			p.Capability.Form = guided
			p.Capability.Revision = r.FormValue("revision")
			if status == http.StatusUnprocessableEntity {
				guided.Error = err.Error()
			}
		}
		p.Error = ErrCapabilityUnavailable.Error()
		if probeMessage != "" {
			p.Error = ""
		}
		for _, known := range []error{ErrCapabilityConflict, ErrCapabilityInvalid, ErrCapabilityUnverified, ErrCapabilityMigration} {
			if errors.Is(err, known) {
				p.Error = known.Error()
			}
		}
		if guided != nil && guided.Error != "" {
			p.Error = ""
		}
		w.WriteHeader(status)
	}
	s.render(w, r, p)
	return nil
}
func validCapability(id string) bool {
	switch id {
	case "memory", "embedding", "mcp", "skills", "execution", "computer":
		return true
	}
	return false
}
