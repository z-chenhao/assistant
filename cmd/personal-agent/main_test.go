package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestControlDiagnosticsSurviveUnavailableExecutionRuntime(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ERII_DATA_ROOT", filepath.Join(root, "data"))
	cfg := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(cfg, []byte("version: 1\njavascript:\n  command: /missing/node\n  script: /missing/sidecar.mjs\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, command := range [][]string{{"status"}, {"ops", "health"}, {"model", "list"}} {
		var out, errOut bytes.Buffer
		if err := run(context.Background(), append([]string{"-config", cfg}, command...), strings.NewReader(""), &out, &errOut); err != nil {
			t.Fatal("control command depended on execution runtime", command, err)
		}
		if !json.Valid(out.Bytes()) {
			t.Fatal("invalid diagnostics", out.String())
		}
	}
}

func TestBoundaryBarriersRequireExplicitFixtureAndRejectAmbiguity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	t.Setenv("ERII_DATA_ROOT", root)
	for _, args := range [][]string{
		{"-test-main-barrier", "before_worker_generation=/missing", "status"},
		{"-test-bridge", "recall", "-test-main-barrier", "missing-equals", "status"},
		{"-test-bridge", "recall", "-test-main-barrier", "=file", "status"},
		{"-test-bridge", "recall", "-test-main-barrier", "boundary=", "status"},
		{"-test-bridge", "recall", "-test-main-barrier", "boundary=one", "-test-main-barrier", "boundary=two", "status"},
		{"-test-bridge", "recall", "-test-main-hold", "boundary", "-test-main-barrier", "boundary=file", "status"},
	} {
		var out, errOut bytes.Buffer
		if err := run(context.Background(), args, strings.NewReader(""), &out, &errOut); err == nil {
			t.Fatal("invalid barrier accepted", args)
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatal("invalid test barrier created product state", err)
		}
	}
}

const metadataJSON = `{"name":"local","provider_kind":"openai_compatible","endpoint":"http://localhost:8000/v1","wire_api":"openai_completions","model_id":"fixture","context_window":8192,"max_output_tokens":1024,"compatibility":{"tool_result_format":"openai"},"capabilities":{"text_input":true}}`

func TestMetadataCLIRejectsUnsafeInputWithoutValues(t *testing.T) {
	t.Setenv("ERII_DATA_ROOT", t.TempDir())
	for _, input := range []string{
		`{"api_key":"SYNTHETIC_REDACTION_MARKER"}`,
		strings.Replace(metadataJSON, `"text_input":true`, `"text_input":true,"refresh_token":"SYNTHETIC_REDACTION_MARKER"`, 1),
		strings.Replace(metadataJSON, "http://localhost:8000/v1", "http://user:SYNTHETIC_REDACTION_MARKER@host/v1", 1),
		metadataJSON + ` {}`, metadataJSON + strings.Repeat(" ", 65536),
		strings.Replace(metadataJSON, `"name":"local"`, `"name":"local","status":"ready"`, 1),
	} {
		var out, errOut bytes.Buffer
		err := run(context.Background(), []string{"model", "add"}, strings.NewReader(input), &out, &errOut)
		if err == nil || out.Len() != 0 || strings.Contains(err.Error()+errOut.String(), "SYNTHETIC_REDACTION_MARKER") {
			t.Fatalf("unsafe CLI result: %v, %s", err, &out)
		}
	}
}

func TestMetadataCLIAddListAndDiagnostics(t *testing.T) {
	t.Setenv("ERII_DATA_ROOT", t.TempDir())
	invoke := func(args ...string) json.RawMessage {
		t.Helper()
		var out, errOut bytes.Buffer
		if err := run(context.Background(), args, strings.NewReader(metadataJSON), &out, &errOut); err != nil {
			t.Fatal(err)
		}
		if errOut.Len() != 0 {
			t.Fatal(errOut.String())
		}
		return out.Bytes()
	}
	if string(invoke("model", "list")) != "[]\n" {
		t.Fatal("implicit model")
	}
	added := invoke("model", "add")
	var list []json.RawMessage
	if err := json.Unmarshal(invoke("model", "list"), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || !bytes.Equal(bytes.TrimSpace(added), list[0]) {
		t.Fatal("list differs from add")
	}
	var status struct {
		Readiness string  `json:"readiness"`
		Ingress   bool    `json:"agent_ingress_enabled"`
		Active    *string `json:"active_model_profile_id"`
	}
	if err := json.Unmarshal(invoke("status"), &status); err != nil {
		t.Fatal(err)
	}
	if status.Readiness != "setup_required" || status.Ingress || status.Active != nil {
		t.Fatal("metadata activated model")
	}
}
