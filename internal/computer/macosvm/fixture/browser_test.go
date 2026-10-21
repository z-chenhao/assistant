package fixture

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrowserPeerIsExplicitLoopbackAndNeverFollowsRedirect(t *testing.T) {
	for _, raw := range []string{"https://127.0.0.1:1234/call", "http://example.com:1234/call", "http://127.0.0.1/call", "http://127.0.0.1:80/call", "http://user@127.0.0.1:1234/call", "http://127.0.0.1:1234/call?q=1"} {
		if browserPeerURL(raw) {
			t.Fatalf("accepted %s", raw)
		}
	}
	if !browserPeerURL("http://127.0.0.1:1234/call") {
		t.Fatal("loopback rejected")
	}
	called := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer target.Close()
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer peer.Close()
	s := &Server{browserEndpoint: peer.URL}
	result, _ := json.Marshal(s.browserCall(`{"method":"inventory","args":{}}`))
	if string(result) != `{"error":"UNKNOWN_SIDE_EFFECT"}` || called {
		t.Fatalf("redirect: %s called=%v", result, called)
	}
}
