package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/z-chenhao/erii/internal/task"
)

type TaskReader interface {
	List(context.Context, task.ListQuery) (task.List, error)
	Detail(context.Context, string, string) (task.Detail, error)
}
type shownArtifact struct{ Ref, Name string }
type taskView struct {
	Query     task.ListQuery
	List      task.List
	Detail    *task.Detail
	Messages  []shownMessage
	Approvals []approvalView
	Artifacts []shownArtifact
}

func taskURL(ref string) string { return "/tasks/" + url.PathEscape(ref) }
func conversationURL(sequence int64) string {
	if sequence <= 0 {
		return "/"
	}
	return "/?before=" + strconv.FormatInt(sequence+1, 10) + "#message-" + strconv.FormatInt(sequence, 10)
}
func tasksQuery(r *http.Request) (task.ListQuery, error) {
	q := task.ListQuery{View: "open"}
	for key, values := range r.URL.Query() {
		if len(values) != 1 {
			return q, task.ErrInvalid
		}
		switch key {
		case "view":
			q.View = values[0]
		case "q":
			q.Query = values[0]
		case "cursor":
			q.Cursor = values[0]
		default:
			return q, task.ErrInvalid
		}
	}
	if q.View == "" {
		q.View = "open"
	}
	return q, q.Validate()
}
func taskListURL(v *taskView, field, value string) string {
	q := url.Values{}
	if v != nil {
		q.Set("view", v.Query.View)
		q.Set("q", v.Query.Query)
	}
	q.Set(field, value)
	if field != "cursor" {
		q.Del("cursor")
	}
	return "/tasks?" + q.Encode()
}
func (s *Server) loadTasks(ctx context.Context, r *http.Request, p *page) error {
	if s.Tasks == nil {
		return errors.New("task workspace unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	v := &taskView{}
	p.Tasks = v
	if r.URL.Path == "/tasks" || r.URL.Path == "/progress" {
		q, err := tasksQuery(r)
		if err != nil {
			return err
		}
		v.Query = q
		v.List, err = s.Tasks.List(ctx, q)
		return err
	}
	ref := strings.TrimPrefix(r.URL.Path, "/tasks/")
	for key, values := range r.URL.Query() {
		if key != "history" || len(values) != 1 {
			return task.ErrInvalid
		}
	}
	d, err := s.Tasks.Detail(ctx, ref, r.URL.Query().Get("history"))
	if err != nil {
		return err
	}
	v.Detail = &d
	for _, ref := range d.Artifacts {
		descriptor, e := s.Core.DescribeArtifact(ctx, ref)
		if e == nil {
			v.Artifacts = append(v.Artifacts, shownArtifact{Ref: ref, Name: descriptor.Name})
		}
	}
	if len(d.ApprovalIDs) > 0 {
		approvals, e := s.Core.WebApprovals(ctx)
		if e != nil {
			return e
		}
		wanted := map[string]bool{}
		for _, id := range d.ApprovalIDs {
			wanted[id] = true
		}
		for _, a := range approvals {
			if wanted[a.ID] && a.Status == "pending" {
				card := approvalFor(a)
				card.ReturnTo = taskURL(d.TaskRef)
				v.Approvals = append(v.Approvals, card)
			}
		}
	}
	for _, m := range d.Messages {
		shown, err := s.showMessage(ctx, m, nil)
		if err != nil {
			return err
		}
		v.Messages = append(v.Messages, shown)
	}
	return nil
}
