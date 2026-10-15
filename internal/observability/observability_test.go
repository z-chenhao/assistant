package observability

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorrelatedLocalSpansAndLevelFiltering(t *testing.T) {
	dir := t.TempDir()
	telemetry, err := Open(dir, slog.LevelInfo)
	if err != nil {
		t.Fatal(err)
	}
	ctx, parent, err := telemetry.Start(context.Background(), "startup")
	if err != nil {
		t.Fatal(err)
	}
	_, child, err := telemetry.Start(ctx, "database")
	if err != nil {
		t.Fatal(err)
	}
	parent.Logger.Debug("filtered")
	parent.Logger.Info("started")
	child.End(errors.New("SECRET_SENTINEL"))
	parent.End(nil)
	if err := telemetry.Close(); err != nil {
		t.Fatal(err)
	}
	logs := readJSONLines(t, filepath.Join(dir, "logs", "app.jsonl"))
	spans := readJSONLines(t, filepath.Join(dir, "logs", "traces.jsonl"))
	if len(logs) != 1 || len(spans) != 2 {
		t.Fatalf("got %d logs and %d spans", len(logs), len(spans))
	}
	if logs[0]["trace_id"] != spans[0]["trace_id"] || spans[0]["trace_id"] != spans[1]["trace_id"] || spans[0]["parent_span_id"] != spans[1]["span_id"] {
		t.Fatal("missing correlation")
	}
	if len(spans[0]["trace_id"].(string)) != 32 || len(spans[0]["span_id"].(string)) != 16 {
		t.Fatal("invalid ID length")
	}
	if spans[0]["status"] != "error" || spans[1]["status"] != "ok" {
		t.Fatal("incorrect span outcome")
	}
}

func readJSONLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var lines []map[string]any
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "SECRET_SENTINEL") {
			t.Fatal("error payload leaked")
		}
		var line map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}
