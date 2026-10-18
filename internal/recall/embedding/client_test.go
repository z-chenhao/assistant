package embedding

import (
	"context"
	"fmt"
	"github.com/z-chenhao/erii/internal/recall"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddingContractAndSanitizedFailure(t *testing.T) {
	for _, body := range []string{`{"model":"test","data":[{"index":0,"embedding":[1,0]}]}`, `{"model":"wrong","data":[{"index":0,"embedding":[1,0]}]}`, `{"model":"test","data":[{"index":0,"embedding":[1]}]}`, `{"model":"test","data":[{"index":0,"embedding":[0,0]}]}`, `{"model":"test","data":[{"index":2,"embedding":[1,0]}]}`, `SYNTHETIC_SECRET`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/embeddings" || r.Method != "POST" {
					t.Error(r.URL, r.Method)
				}
				fmt.Fprint(w, body)
			}))
			defer server.Close()
			c := Client{Config: Config{Provider: "openai_compatible", Endpoint: server.URL + "/v1", Model: "test", Dimensions: 2}}
			v, e := c.Embed(context.Background(), "tea")
			if strings.Contains(body, `"embedding":[1,0]`) && !strings.Contains(body, `wrong`) && !strings.Contains(body, `"index":2`) {
				if e != nil || len(v) != 2 {
					t.Fatal(v, e)
				}
			} else if e != recall.ErrUnavailable || v != nil {
				t.Fatal(v, e)
			}
		})
	}
}
func TestEmbeddingRedirectAndConfigDrift(t *testing.T) {
	calls := 0
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer destination.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 307) }))
	defer s.Close()
	c := Client{Config: Config{Provider: "openai_compatible", Endpoint: s.URL, Model: "test", Dimensions: 2}}
	if _, e := c.Embed(context.Background(), "tea"); e == nil || calls != 0 {
		t.Fatal(e, calls)
	}
	id := c.Identity()
	c.Config.Model = "changed"
	if id == c.Identity() {
		t.Fatal("identity drift undetected")
	}
	for _, endpoint := range []string{"http://remote.example/v1", "https://user:secret@example.com/v1", "https://example.com/v1?key=secret"} {
		c.Config.Endpoint = endpoint
		if c.Config.Validate() == nil {
			t.Fatal(endpoint)
		}
	}
}
