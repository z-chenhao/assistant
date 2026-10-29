package web

import (
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/z-chenhao/collie/internal/llm"
)

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	if legacyPageRedirect(w, r) {
		return
	}
	if _, view, _ := pageRoute(r.URL.Path); view != "" {
		s.show(w, r)
		return
	}
	switch r.URL.Path {
	case "/api/developer/traces":
		s.traceAPI(w, r, "traces")
	case "/api/developer/observations":
		s.traceAPI(w, r, "observations")
	case "/api/developer/observation-body":
		s.traceAPI(w, r, "body")
	case "/api/developer/rounds":
		s.contextAPI(w, r, "rounds")
	case "/api/developer/context":
		s.contextAPI(w, r, "context")
	case "/api/developer/context-diff":
		s.contextAPI(w, r, "diff")
	case "/api/developer/executions":
		s.inspectionAPI(w, r, false)
	case "/api/developer/relationships":
		s.inspectionAPI(w, r, true)
	case "/api/updates":
		s.updates(w, r)
	case "/api/conversation":
		before, e := conversationCursor(r)
		if e != nil {
			fail(w, 400)
			return
		}
		p, e := s.Core.Conversation(r.Context(), before)
		if e != nil {
			fail(w, 503)
			return
		}
		if p.Before != 0 {
			w.Header().Set("X-Collie-Before", strconv.FormatInt(p.Before, 10))
		}
		jsonResult(w, p.Messages)
	case "/api/progress":
		a, e := s.Core.Activity(r.Context())
		if e != nil {
			fail(w, 503)
			return
		}
		jsonResult(w, a)
	case "/api/status":
		a, e := s.Core.WebStatus(r.Context())
		if e != nil {
			fail(w, 503)
			return
		}
		jsonResult(w, a)
	case "/api/auth":
		s.mu.Lock()
		defer s.mu.Unlock()
		jsonResult(w, struct {
			llm.AuthState
			AccountID string `json:"account_id,omitempty"`
		}{s.auth, s.accountID})
	case "/artifact":
		ref := r.URL.Query().Get("ref")
		d, e := s.Core.DescribeArtifact(r.Context(), ref)
		if e != nil {
			fail(w, 404)
			return
		}
		src, e := s.Core.OpenArtifact(r.Context(), ref)
		if e != nil {
			fail(w, 404)
			return
		}
		defer src.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": d.Name}))
		_, _ = io.Copy(w, src)
	default:
		fail(w, 404)
	}
}
func (s *Server) post(w http.ResponseWriter, r *http.Request) error {
	switch r.URL.Path {
	case "/api/message":
		return s.acceptJSONMessage(w, r)
	case "/message":
		return s.acceptFormMessage(w, r)
	case "/upload":
		return s.uploadArtifact(w, r)
	}
	if e := r.ParseForm(); e != nil {
		return e
	}
	switch r.URL.Path {
	case "/capability/configure":
		return s.capabilityAction(w, r)
	case "/approval":
		if err := s.decideApproval(r); err != nil {
			return err
		}
		if r.Header.Get("Accept") == "application/json" {
			jsonResult(w, struct {
				Accepted bool `json:"accepted"`
			}{true})
			return nil
		}
		http.Redirect(w, r, safePageReturn(r.FormValue("return_to")), 303)
		return nil
	case "/model/add":
		p, err := s.addModel(r)
		if err != nil {
			return err
		}
		http.Redirect(w, r, modelURL(p.ProfileID)+"?notice=added", 303)
		return nil
	case "/model/test":
		if _, e := s.Control.Test(r.Context(), r.FormValue("profile")); e != nil {
			return e
		}
		http.Redirect(w, r, modelURL(r.FormValue("profile"))+"?notice=tested", 303)
		return nil
	case "/model/select":
		if err := s.selectModel(r); err != nil {
			return err
		}
		http.Redirect(w, r, modelURL(r.FormValue("profile"))+"?notice=active", 303)
		return nil
	case "/notification":
		if err := s.preferNotification(r); err != nil {
			return err
		}
		http.Redirect(w, r, "/settings/notifications?notice=saved", 303)
		return nil
	case "/auth/login":
		if err := s.startLogin(r); err != nil {
			return err
		}
	case "/auth/logout":
		if s.Accounts == nil {
			return llm.ErrNoBridge
		}
		if _, err := s.Accounts.Logout(r.Context(), r.FormValue("account")); err != nil {
			return err
		}
		s.mu.Lock()
		if s.accountID == r.FormValue("account") {
			s.auth = llm.AuthState{}
			s.accountID = ""
		}
		s.mu.Unlock()
	case "/auth/cancel":
		s.mu.Lock()
		cancel := s.authCancel
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	default:
		fail(w, 404)
		return nil
	}
	target := "/"
	if strings.HasPrefix(r.URL.Path, "/auth/") {
		target = "/settings/models/new?provider=chatgpt"
		if id := r.FormValue("profile"); id != "" {
			target = modelURL(id)
		}
	}
	http.Redirect(w, r, safePageReturn(target), 303)
	return nil
}
