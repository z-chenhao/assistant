package web

import (
	"context"
	"github.com/z-chenhao/assistant/internal/computer"
	"net/http/httptest"
	"strings"
	"testing"
)

type previewReader struct{ calls int }

func (p *previewReader) ComputerStatus(context.Context) (computer.Status, error) {
	p.calls++
	return computer.Status{Phase: "ready", ComputerID: "PRIVATE_VM_ID", Epoch: 4}, nil
}
func (p *previewReader) ComputerFrame(context.Context) ([]byte, error) {
	p.calls++
	return []byte("fixture-frame"), nil
}
func (p *previewReader) ComputerActivity(context.Context) ([]computer.Activity, error) {
	return []computer.Activity{{ID: "op", Action: "getScreenshot", Status: "succeeded"}}, nil
}
func TestComputerPreviewRequiresCurrentAuthAndNeverExposesNativeIdentity(t *testing.T) {
	core := &updatesCore{allowed: true}
	reader := &previewReader{}
	server := &Server{Config: Config{TokenRef: "credential:test"}, Core: core, Secrets: fakeSecrets{}, Computer: reader}
	request := func(method, path string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, nil)
		if auth {
			r.Header.Set("Authorization", "Bearer private-token-at-least-24-bytes")
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/api/computer/status", "/api/computer/frame"} {
		if w := request("GET", path, false); w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	if reader.calls != 0 {
		t.Fatal("unauthenticated preview reached Computer")
	}
	w := request("GET", "/api/computer/status", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"phase":"ready"`) || strings.Contains(w.Body.String(), "PRIVATE_VM_ID") {
		t.Fatal(w.Code, w.Body.String())
	}
	frame := request("GET", "/api/computer/frame", true)
	if frame.Code != 200 || frame.Header().Get("Content-Type") != "image/png" || frame.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal(frame.Code, frame.Header())
	}
	core.allowed = false
	before := reader.calls
	if w := request("GET", "/api/computer/frame", true); w.Code != 403 || reader.calls != before {
		t.Fatal("revoked browser still reads frames", w.Code)
	}
}
