package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/artifact"
	"github.com/z-chenhao/assistant/internal/evidence"
)

func TestEvidenceCLIReadsOriginalAcrossReopenedBoundedPages(t *testing.T) {
	root := t.TempDir()
	t.Setenv("STEWARD_DATA_ROOT", filepath.Join(root, "data"))
	text := strings.Repeat("original 中文 🧭\n", 1000)
	var saved, errOut bytes.Buffer
	if err := run(context.Background(), []string{"artifact", "save", "original.txt", "text/plain"}, strings.NewReader(text), &saved, &errOut); err != nil {
		t.Fatal(err, errOut.String())
	}
	var descriptor artifact.Descriptor
	if json.Unmarshal(saved.Bytes(), &descriptor) != nil || descriptor.Ref == "" {
		t.Fatal("save output", saved.String())
	}
	config := filepath.Join(root, "broken-execution.yaml")
	if err := os.WriteFile(config, []byte("version: 1\njavascript:\n  command: /missing/node\n  script: /missing/script.mjs\n"), 0600); err != nil {
		t.Fatal(err)
	}
	q := evidence.Request{Ref: descriptor.Ref}
	var joined strings.Builder
	for {
		input, _ := json.Marshal(q)
		var out, errOut bytes.Buffer
		if err := run(context.Background(), []string{"-config", config, "evidence", "read"}, bytes.NewReader(input), &out, &errOut); err != nil {
			t.Fatal(err, errOut.String())
		}
		var page evidence.Page
		if json.Unmarshal(out.Bytes(), &page) != nil || page.Ref != descriptor.Ref || page.Kind != "artifact" || page.Bytes > 4096 {
			t.Fatal(out.String())
		}
		joined.WriteString(page.Text)
		if page.NextOffset == nil {
			break
		}
		q.Offset = *page.NextOffset
	}
	if joined.String() != text {
		t.Fatal("CLI changed or omitted original bytes")
	}
	for _, input := range []string{`{"ref":"current","agent_id":"forged"}`, `{"ref":"memory:unsupported"}`, `{"ref":"artifact:missing"}`} {
		var out, errOut bytes.Buffer
		if err := run(context.Background(), []string{"-config", config, "evidence", "read"}, strings.NewReader(input), &out, &errOut); err == nil || out.Len() != 0 {
			t.Fatal("invalid or inaccessible evidence accepted", input, out.String(), err)
		}
	}
}
