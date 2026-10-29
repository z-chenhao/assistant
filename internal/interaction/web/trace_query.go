package web

import (
	"fmt"
	"github.com/z-chenhao/erii/internal/observability"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func traceQuery(r *http.Request) (*traceView, error) {
	values := r.URL.Query()
	v := &traceView{Tab: "overview", Field: "output"}
	legacy := url.Values{}
	for key, list := range values {
		if len(list) != 1 {
			return nil, observability.ErrInvalid
		}
		switch key {
		case "session":
			v.Query.Session = list[0]
		case "obs":
			v.Node = list[0]
		case "nodes_after":
			v.After = list[0]
		case "tab":
			v.Tab = list[0]
		case "field":
			v.Field = list[0]
		case "diff_after":
			n, err := strconv.Atoi(list[0])
			if err != nil || n < 0 || n > 1<<30 {
				return nil, observability.ErrInvalid
			}
			v.DiffAfter = n
		case "offset":
			n, err := strconv.Atoi(list[0])
			if err != nil || n < 0 || n > 1<<30 {
				return nil, observability.ErrInvalid
			}
			v.Offset = n
		case "role", "status", "since", "until", "cursor", "run", "task", "relations_after", "limit":
			legacy[key] = list
		default:
			return nil, observability.ErrInvalid
		}
	}
	copy := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = legacy.Encode()
	copy.URL = &u
	old, err := inspectionQuery(copy)
	if err != nil {
		return nil, err
	}
	v.Query.ExecutionQuery = old.Query
	v.Run = old.RunID
	v.Task = old.TaskRef
	if v.Query.Validate() != nil || len(v.Node) > 300 || len(v.After) > 512 {
		return nil, observability.ErrInvalid
	}
	switch v.Tab {
	case "overview", "context", "diff", "wire", "metadata":
	default:
		return nil, observability.ErrInvalid
	}
	switch v.Field {
	case "input", "output", "wire":
	default:
		return nil, observability.ErrInvalid
	}
	if v.Run == "" && (v.Node != "" || v.After != "" || v.Offset > 0) {
		return nil, observability.ErrInvalid
	}
	return v, nil
}

func traceURL(v *traceView, field string, value any) string {
	q := url.Values{}
	if v.Query.Session != "" {
		q.Set("session", v.Query.Session)
	}
	if v.Query.Role != "" {
		q.Set("role", v.Query.Role)
	}
	if v.Query.Status != "" {
		q.Set("status", v.Query.Status)
	}
	if !v.Query.Since.IsZero() {
		q.Set("since", v.Query.Since.Format(time.RFC3339Nano))
	}
	if !v.Query.Until.IsZero() {
		q.Set("until", v.Query.Until.Format(time.RFC3339Nano))
	}
	if v.Query.Cursor != "" {
		q.Set("cursor", v.Query.Cursor)
	}
	if v.Run != "" {
		q.Set("run", v.Run)
	}
	if v.Task != "" {
		q.Set("task", v.Task)
	}
	if v.Node != "" {
		q.Set("obs", v.Node)
	}
	if v.After != "" {
		q.Set("nodes_after", v.After)
	}
	if v.Tab != "overview" {
		q.Set("tab", v.Tab)
	}
	if v.DiffAfter > 0 {
		q.Set("diff_after", strconv.Itoa(v.DiffAfter))
	}
	if v.Offset > 0 {
		q.Set("offset", strconv.Itoa(v.Offset))
		q.Set("field", v.Field)
	}
	text := fmt.Sprint(value)
	switch x := value.(type) {
	case *int:
		text = strconv.Itoa(*x)
	case *int64:
		text = strconv.FormatInt(*x, 10)
	}
	switch field {
	case "obs":
		q.Set("obs", text)
		q.Del("offset")
		q.Del("field")
	case "run", "task":
		q.Del("run")
		q.Del("task")
		q.Del("obs")
		q.Del("offset")
		q.Del("field")
		q.Del("nodes_after")
		q.Del("tab")
		q.Set(field, text)
	case "tab":
		q.Set("tab", text)
		q.Del("offset")
		q.Del("field")
	case "session":
		q = url.Values{"session": {text}}
		q.Del("run")
		q.Del("task")
		q.Del("obs")
		q.Del("cursor")
		q.Del("nodes_after")
		q.Del("tab")
		q.Del("offset")
		q.Del("field")
	case "input", "output", "wire":
		q.Set("field", field)
		q.Set("offset", text)
	default:
		q.Set(field, text)
	}
	return "/developer/observability?" + q.Encode()
}
