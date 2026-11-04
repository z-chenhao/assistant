package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/z-chenhao/assistant/internal/database"
)

type DatabaseReader interface {
	Database(context.Context, database.Query) (database.Page, error)
}

type databaseView struct {
	Query database.Query
	Page  database.Page
}

func databaseQuery(r *http.Request) (database.Query, error) {
	v := database.Query{Collection: "executions"}
	values := r.URL.Query()
	for key, items := range values {
		if len(items) != 1 || key != "collection" && key != "ref" && key != "cursor" && key != "limit" {
			return v, database.ErrInvalid
		}
	}
	if value, exists := values["collection"]; exists {
		v.Collection = value[0]
	}
	v.Ref, v.Cursor = values.Get("ref"), values.Get("cursor")
	if value, exists := values["limit"]; exists {
		var err error
		v.Limit, err = strconv.Atoi(value[0])
		if err != nil || v.Limit < 1 {
			return v, database.ErrInvalid
		}
	}
	return v, v.Validate()
}

func (s *Server) loadDatabase(ctx context.Context, r *http.Request) (*databaseView, error) {
	q, err := databaseQuery(r)
	if err != nil {
		return nil, err
	}
	if s.Database == nil {
		return nil, database.ErrUnavailable
	}
	p, err := s.Database.Database(ctx, q)
	return &databaseView{Query: q, Page: p}, err
}

func (s *Server) databaseAPI(w http.ResponseWriter, r *http.Request) {
	v, err := s.loadDatabase(r.Context(), r)
	if err != nil {
		status := 503
		if errors.Is(err, database.ErrInvalid) {
			status = 400
		} else if errors.Is(err, database.ErrNotFound) {
			status = 404
		}
		fail(w, status)
		return
	}
	jsonResult(w, v.Page)
}

func databaseURL(collection, ref, cursor string) string {
	q := url.Values{"collection": {collection}}
	if ref != "" {
		q.Set("ref", ref)
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	return "/developer/database?" + q.Encode()
}
func databaseLink(ref string) string {
	if !database.ValidRef(ref) {
		return ""
	}
	if strings.HasPrefix(ref, "task:") {
		return taskURL(ref)
	}
	return databaseURL(database.Collection(ref), ref, "")
}
func databaseLabel(kind string) string {
	switch kind {
	case "executions":
		return "Executions"
	case "messages":
		return "Messages"
	case "memories":
		return "Memory"
	case "artifacts":
		return "Artifacts"
	}
	return "Database"
}
func databaseTrace(ref string) string {
	if database.ValidRef(ref) && strings.HasPrefix(ref, "run:") {
		return "/developer/observability?" + url.Values{"run": {strings.TrimPrefix(ref, "run:")}}.Encode()
	}
	return ""
}
