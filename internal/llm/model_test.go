package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"testing"
)

func validInput() ProfileInput {
	return ProfileInput{Name: "fixture", ProviderKind: "openai_compatible", Endpoint: "http://localhost:8000/v1", WireAPI: "openai_completions", ModelID: "fixture", ContextWindow: 8192, MaxOutputTokens: 1024, Compatibility: Compatibility{ToolResultFormat: "openai"}, Capabilities: Capabilities{TextInput: true}}
}

func TestProfileValidationBoundaries(t *testing.T) {
	if err := validInput().Validate(); err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*ProfileInput){
		"name":         func(p *ProfileInput) { p.Name = " " },
		"model":        func(p *ProfileInput) { p.ModelID = "" },
		"provider":     func(p *ProfileInput) { p.ProviderKind = "fake" },
		"wire":         func(p *ProfileInput) { p.WireAPI = "unknown" },
		"context":      func(p *ProfileInput) { p.ContextWindow = 0 },
		"output":       func(p *ProfileInput) { p.MaxOutputTokens = 8193 },
		"text":         func(p *ProfileInput) { p.Capabilities.TextInput = false },
		"format":       func(p *ProfileInput) { p.Compatibility.ToolResultFormat = "unknown" },
		"effort":       func(p *ProfileInput) { p.ReasoningEffort = "unsupported" },
		"empty-effort": func(p *ProfileInput) { p.Capabilities.ReasoningEfforts = []string{""} },
		"temperature":  func(p *ProfileInput) { v := 2.01; p.GenerationDefaults.Temperature = &v },
		"top_p":        func(p *ProfileInput) { v := -0.1; p.GenerationDefaults.TopP = &v },
		"nan":          func(p *ProfileInput) { v := math.NaN(); p.GenerationDefaults.TopP = &v },
		"infinity":     func(p *ProfileInput) { v := math.Inf(1); p.GenerationDefaults.Temperature = &v },
		"account":      func(p *ProfileInput) { p.ProviderKind = "openai_account" },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			p := validInput()
			change(&p)
			if p.Validate() == nil {
				t.Fatal("invalid profile accepted")
			}
		})
	}
	for _, endpoint := range []string{"", "file:///tmp/model", "https://user:SYNTHETIC_VALUE@host/v1", "https://host/v1?key=SYNTHETIC_VALUE", "https://host/v1?", "https://host/#SYNTHETIC_VALUE", "https://host/#", "http://", "http://host/path with spaces", "http://host:bad"} {
		p := validInput()
		p.Endpoint = endpoint
		if err := p.Validate(); err == nil || strings.Contains(err.Error(), "SYNTHETIC_VALUE") {
			t.Fatalf("invalid or unsafe URL result: %v", err)
		}
	}
	p := validInput()
	p.ProviderKind, p.Endpoint, p.WireAPI, p.AuthConnectionID = "openai_account", "", "openai_responses", "opaque-host-ref"
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalJSONBeforeEndpointHash(t *testing.T) {
	first, err := CanonicalJSON(map[string]any{"z": true, "a": map[string]any{"y": 1.0, "b": 0.0}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := CanonicalJSON(map[string]any{"a": map[string]any{"b": 0, "y": 1}, "z": true})
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != `{"a":{"b":0,"y":1},"z":true}` || string(first) != string(second) {
		t.Fatalf("noncanonical JSON: %s, %s", first, second)
	}
	zero, err := CanonicalJSON(map[string]float64{"value": math.Copysign(0, -1)})
	if err != nil || string(zero) != `{"value":0}` {
		t.Fatal("negative zero was not normalized")
	}
	hash, err := EndpointHash("http://localhost:8000/v1")
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256([]byte(`{"endpoint":"http://localhost:8000/v1"}`))
	if hash != hex.EncodeToString(want[:]) {
		t.Fatal("hash did not use canonical endpoint JSON")
	}
	other, err := EndpointHash("http://localhost:8001/v1")
	if err != nil || other == hash {
		t.Fatal("endpoint drift not hashed")
	}
	if _, err := CanonicalJSON(math.NaN()); err == nil {
		t.Fatal("hashed invalid JSON")
	}
}

type canceledAuthWriter struct{ cancel context.CancelFunc }

func (w canceledAuthWriter) InsertAuth(context.Context, AuthConnection) error {
	w.cancel()
	return ErrStorage
}

type cleanupSecrets struct {
	ref     string
	deleted bool
}

func (s *cleanupSecrets) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("unused")
}
func (s *cleanupSecrets) Put(_ context.Context, ref string, _ []byte) error { s.ref = ref; return nil }
func (s *cleanupSecrets) Delete(ctx context.Context, ref string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.deleted = ref == s.ref
	return nil
}

func TestAuthRollbackCleanupOutlivesCanceledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	secrets := &cleanupSecrets{}
	_, err := RegisterAuth(ctx, canceledAuthWriter{cancel}, secrets, AuthInput{Provider: "local", AuthKind: "api_key"}, []byte("fixture"))
	if !errors.Is(err, ErrStorage) || errors.Is(err, ErrSecretCleanup) || !secrets.deleted {
		t.Fatalf("credential not cleaned after cancellation: %v", err)
	}
}
