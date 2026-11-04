package web

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/database"
)

type metadataReader struct {
	queries []database.Query
	failure error
}

func (m *metadataReader) Database(_ context.Context, q database.Query) (database.Page, error) {
	m.queries = append(m.queries, q)
	return database.Page{Collection: q.Collection, Items: []database.Record{{Ref: "run:local", Status: "completed", CreatedAt: "2026-11-04T01:00:00Z", Fields: []database.Field{{Name: "Name", Value: `<img src=x onerror="private()">`}}, Links: []database.Link{{Relation: "Attachment", Ref: "artifact:local"}}}}, Next: "opaque-next"}, m.failure
}

func TestDatabaseRoutesRequireOwnerAccessAndRejectQueriesAndWrites(t *testing.T) {
	reader := &metadataReader{}
	core := &updatesCore{allowed: true, status: "idle"}
	s := &Server{Config: Config{TokenRef: "credential:test"}, Core: core, Secrets: fakeSecrets{}, Database: reader}
	request := func(method, path string, auth bool) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, nil)
		if auth {
			r.Header.Set("Authorization", "Bearer private-token-at-least-24-bytes")
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/developer/database", "/api/developer/database"} {
		if w := request("GET", path, false); w.Code != 401 {
			t.Fatal("unauthorized metadata inspection", w.Code)
		}
	}
	if len(reader.queries) != 0 {
		t.Fatal("unauthorized request reached the reader")
	}
	for _, path := range []string{"/api/developer/database?collection=auth_connection", "/api/developer/database?sql=select+1", "/api/developer/database?agent=other", "/api/developer/database?collection=messages&collection=memories", "/api/developer/database?limit=0", "/api/developer/database?limit=51", "/api/developer/database?limit=oops", "/developer/database?collection=messages&ref=memory:private"} {
		if w := request("GET", path, true); w.Code != 400 {
			t.Fatal("invalid query reached inspection", path, w.Code)
		}
	}
	if len(reader.queries) != 0 {
		t.Fatal("invalid query reached storage")
	}
	for _, path := range []string{"/developer/database", "/api/developer/database"} {
		if w := request("POST", path, true); w.Code < 400 {
			t.Fatal("metadata view accepted a write")
		}
	}
	if len(reader.queries) != 0 {
		t.Fatal("write request reached storage")
	}
	w := request("GET", "/developer/database?collection=executions&ref=run:local", true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, text := range []string{"Database", "Metadata and relationships", "&lt;img src=x", "collection=artifacts", "Inspect execution", "Read-only metadata"} {
		if !strings.Contains(w.Body.String(), text) {
			t.Fatal("missing metadata navigation", text)
		}
	}
	if strings.Contains(w.Body.String(), `<img src=x`) || strings.Contains(w.Body.String(), `id="composer-root"`) {
		t.Fatal("database page executes stored content or depends on the model composer")
	}
	if w := request("GET", "/api/developer/database?collection=messages&limit=2", true); w.Code != 200 || !strings.Contains(w.Body.String(), `"collection":"messages"`) {
		t.Fatal("bounded API projection failed", w.Code)
	}
	before := len(reader.queries)
	core.allowed = false
	if w := request("GET", "/api/developer/database", true); w.Code != 403 || len(reader.queries) != before {
		t.Fatal("revoked binding reached metadata", w.Code)
	}
}

func TestDatabaseFailuresAreSafeAndLocalizable(t *testing.T) {
	s := languageServer()
	reader := &metadataReader{}
	s.Database = reader
	for _, check := range []struct {
		err  error
		code int
	}{{nil, 200}, {database.ErrNotFound, 404}, {database.ErrInvalid, 400}, {errors.New("private sqlite path and credential"), 503}} {
		reader.failure = check.err
		for _, path := range []string{"/developer/database", "/api/developer/database"} {
			w := httptest.NewRecorder()
			s.ServeHTTP(w, languageRequest("GET", path, "zh-CN", ""))
			if w.Code != check.code || strings.Contains(w.Body.String(), "private sqlite") {
				t.Fatal("unsafe metadata failure", check.code, w.Code)
			}
			if check.err == nil && path == "/developer/database" && !strings.Contains(w.Body.String(), "元数据与关联") {
				t.Fatal("metadata UI is not localized")
			}
		}
	}
}
