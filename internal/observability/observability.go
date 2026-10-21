// Package observability provides structured local logs and correlated process
// spans. These diagnostic spans are not a durable RunTrace or a replay ledger.
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

type Telemetry struct {
	Logger             *slog.Logger
	traces             *slog.Logger
	logFile, traceFile io.Closer
}

func Open(dataRoot string, level slog.Level) (*Telemetry, error) {
	return OpenWithLimits(dataRoot, level, 16<<20, 2)
}

func OpenWithLimits(dataRoot string, level slog.Level, maxBytes int64, backups int) (*Telemetry, error) {
	dir := filepath.Join(dataRoot, "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	logFile, err := newWriter(dir, "app.jsonl", maxBytes, backups)
	if err != nil {
		return nil, err
	}
	traceFile, err := newWriter(dir, "traces.jsonl", maxBytes, backups)
	if err != nil {
		return nil, errors.Join(err, logFile.Close())
	}
	return &Telemetry{
		Logger:  slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{Level: level})).With("pid", os.Getpid()),
		traces:  slog.New(slog.NewJSONHandler(traceFile, nil)),
		logFile: logFile, traceFile: traceFile,
	}, nil
}

type spanKey struct{}
type spanContext struct{ traceID, spanID string }

type Span struct {
	Logger         *slog.Logger
	telemetry      *Telemetry
	identity       spanContext
	parentID, name string
	started        time.Time
}

// Start returns an error rather than emitting spans with invalid correlation IDs.
func (t *Telemetry) Start(ctx context.Context, name string) (context.Context, *Span, error) {
	parent, _ := ctx.Value(spanKey{}).(spanContext)
	traceID := parent.traceID
	if traceID == "" {
		var err error
		traceID, err = randomID(16)
		if err != nil {
			return ctx, nil, err
		}
	}
	spanID, err := randomID(8)
	if err != nil {
		return ctx, nil, err
	}
	identity := spanContext{traceID, spanID}
	s := &Span{
		Logger:    t.Logger.With("trace_id", traceID, "span_id", spanID),
		telemetry: t, identity: identity, parentID: parent.spanID, name: name, started: time.Now(),
	}
	return context.WithValue(ctx, spanKey{}, identity), s, nil
}

func randomID(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Span) End(err error) {
	status := "ok"
	if err != nil {
		status = "error"
	}
	// Record outcome, not raw error payloads or configuration values.
	s.telemetry.traces.Info("span", "name", s.name,
		"trace_id", s.identity.traceID, "span_id", s.identity.spanID, "parent_span_id", s.parentID,
		"started_at", s.started.UTC().Format(time.RFC3339Nano), "duration_us", time.Since(s.started).Microseconds(), "status", status)
}

func (t *Telemetry) Close() error {
	return errors.Join(t.logFile.Close(), t.traceFile.Close())
}
