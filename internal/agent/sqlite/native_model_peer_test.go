package sqlite

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/llm/bridge"
)

type nativeCompletionTransport struct{}

func (nativeCompletionTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	body := `{"models":[{"slug":"fixture-model","visibility":"list"}]}`
	if strings.HasSuffix(r.URL.Path, "/responses") {
		body = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"late result\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"fixture\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}
func TestNativeModelCompletionPeer(t *testing.T) {
	selected := false
	for _, arg := range os.Args[1:] {
		selected = selected || arg == "-test.run=TestNativeModelCompletionPeer"
	}
	if !selected {
		return
	}
	if (&bridge.Server{HTTPClient: &http.Client{Transport: nativeCompletionTransport{}}}).Serve(context.Background(), os.Stdin, os.Stdout) != nil {
		os.Exit(2)
	}
	os.Exit(0)
}
func nativeModelPeer(t *testing.T) (string, string) {
	t.Helper()
	command, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return command, "-test.run=TestNativeModelCompletionPeer"
}
