package bridge

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/llm"
)

type catalogTransport string

func (body catalogTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
}

func TestAccountCatalogUsesModelLimitsAndImageCapabilities(t *testing.T) {
	s := &Server{HTTPClient: &http.Client{Transport: catalogTransport(`{"models":[{"slug":"gpt-6-luna","visibility":"list"},{"slug":"gpt-5.6-luna","visibility":"list","context_window":200000,"max_output_tokens":96000,"input_modalities":["text"]},{"slug":"new-model","visibility":"list"},{"slug":"hidden-model","visibility":"hide"}]}`)}}
	models, err := s.catalog(context.Background(), llm.Connection{Endpoint: apiURL, AccessToken: "fixture-only"})
	if err != nil || len(models) != 3 {
		t.Fatal(models, err)
	}
	if models[0].ContextWindow != 272000 || models[0].MaxOutputTokens != 128000 || !models[0].ImageInput {
		t.Fatal("Pi chat catalog metadata was lost", models[0])
	}
	if models[1].ContextWindow != 200000 || models[1].MaxOutputTokens != 96000 || models[1].ImageInput {
		t.Fatal("explicit account metadata was overridden", models[1])
	}
	if models[2].ContextWindow != 128000 || models[2].MaxOutputTokens != 16384 {
		t.Fatal("missing metadata fell back to the old 1024 cap", models[2])
	}
}
