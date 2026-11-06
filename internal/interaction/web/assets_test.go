package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOnlyMatchingFontContentVersionsAreCached(t *testing.T) {
	s := &Server{Config: Config{TokenRef: "credential:test"}}
	for _, path := range []string{"/brand/InterVariable.woff2", "/brand/Michroma-Regular.ttf", "/brand/SourceHanSansSC-common.woff2"} {
		versioned, err := fontURL(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, target := range []string{path, path + "?v=old", versioned} {
			r := httptest.NewRequest("GET", "http://127.0.0.1:8080"+target, nil)
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != 200 || w.Body.Len() == 0 {
				t.Fatal(target, w.Code)
			}
			cached := strings.Contains(w.Header().Get("Cache-Control"), "immutable")
			if cached != (target == versioned) {
				t.Fatal(target, w.Header())
			}
			r.Host = "evil.example:8080"
			denied := httptest.NewRecorder()
			s.ServeHTTP(denied, r)
			if denied.Code != 403 || denied.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cache bypassed request guards", denied.Code, denied.Header())
			}
		}
	}
}
