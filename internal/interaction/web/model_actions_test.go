package web

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/llm"
)

type capturingModels struct {
	Models
	input llm.ProfileInput
}

func (m *capturingModels) AddProfile(_ context.Context, input llm.ProfileInput) (llm.Profile, error) {
	m.input = input
	return llm.Profile{}, nil
}

func TestWebModelImageInputIsAnExplicitUnverifiedDeclaration(t *testing.T) {
	for _, value := range []string{"", "yes"} {
		models := &capturingModels{}
		s := &Server{Models: models}
		form := url.Values{"endpoint": {"http://127.0.0.1:8000/v1"}, "model": {"vision"}, "image_input": {value}}
		r := httptest.NewRequest("POST", "/model/add", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if err := s.addModel(r); err != nil {
			t.Fatal(err)
		}
		if models.input.Capabilities.ImageInput != (value == "yes") || !models.input.Capabilities.TextInput {
			t.Fatal(models.input)
		}
		if err := models.input.Validate(); err != nil {
			t.Fatal("invalid candidate", err)
		}
	}
}
