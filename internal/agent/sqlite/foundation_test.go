package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/artifact"
	"github.com/z-chenhao/erii/internal/interaction/web"
	"github.com/z-chenhao/erii/internal/llm"
)

type gatedArtifactReader struct {
	started, release chan struct{}
	once             sync.Once
}

func (r *gatedArtifactReader) Read(p []byte) (int, error) {
	first := false
	r.once.Do(func() { first = true; close(r.started) })
	if !first {
		return 0, io.EOF
	}
	<-r.release
	return copy(p, "staged bytes"), nil
}

func TestArtifactStreamingAllowsStateAndCrossProcessRecovery(t *testing.T) {
	s := openModels(t)
	owner := artifactOwner(t, s)
	reader := &gatedArtifactReader{started: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	unblock := func() { release.Do(func() { close(reader.release) }) }
	defer unblock()
	done := make(chan error, 1)
	go func() {
		_, err := s.Artifacts().Put(context.Background(), owner, artifact.Input{Name: "stream.txt", MediaType: "text/plain"}, reader)
		done <- err
	}()
	select {
	case <-reader.started:
	case <-time.After(3 * time.Second):
		t.Fatal("stream did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var one int
	if err := s.db.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
		t.Fatal("stream monopolized state connection", err)
	}
	// Opening a second state owner performs orphan recovery while the original
	// writer is paused. Its file lock must keep the live staged object intact.
	other, err := Open(ctx, filepath.Dir(s.path))
	if err != nil {
		t.Fatal("recovery blocked or collected live stage", err)
	}
	defer other.Close()
	report, err := other.Artifacts().Recover(ctx)
	if err != nil || report.Objects != 0 {
		t.Fatal("live staging treated as orphan", report, err)
	}
	unblock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("writer failed to publish")
	}
	if scalar(t, s, `SELECT count(*) FROM artifact`) != 1 {
		t.Fatal("missing metadata")
	}
}

func TestWebSurvivesTerminalMainFailureAndAcceptsNextRequest(t *testing.T) {
	s, _, _, actor, provider := loopFixture(t)
	provider.respond = func(request llm.GenerationRequest) (llm.GenerationResult, error) {
		if strings.Contains(request.Messages[len(request.Messages)-1].Text, "fail this") {
			return llm.GenerationResult{}, llm.ProviderUnavailable
		}
		return llm.GenerationResult{Text: "next request succeeded"}, nil
	}
	mustSubmit(t, actor, "first-failure", "fail this")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	failed := make(chan struct{}, 1)
	go func() {
		done <- web.Run(ctx, &web.Server{}, listener, func(ctx context.Context) error {
			err := actor.Drain(ctx)
			var failure *agent.RunFailure
			if errors.As(err, &failure) {
				select {
				case failed <- struct{}{}:
				default:
				}
			}
			return err
		}, func(context.Context) error { return nil })
	}()
	select {
	case <-failed:
	case err := <-done:
		t.Fatal("surface exited", err)
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not fail")
	}
	receipt := mustSubmit(t, actor, "second-request", "please continue")
	deadline := time.After(3 * time.Second)
	for {
		var status string
		if err := s.db.QueryRow(`SELECT status FROM run WHERE run_id=?`, receipt.RunID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "completed" {
			break
		}
		select {
		case err := <-done:
			t.Fatal("model failure stopped web", err)
		case <-deadline:
			t.Fatal("next request not executed")
		case <-time.After(10 * time.Millisecond):
		}
	}
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + "/favicon.ico")
	if err != nil {
		t.Fatal("listener lost after model failure", err)
	}
	response.Body.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("foreground did not join")
	}
}

type traceSpy struct {
	agent.ActorStore
	updates, failures int
}

func (s *traceSpy) Trace(ctx context.Context, c agent.RunClaim, e agent.Event) (agent.Event, error) {
	if e.Kind == "message_update" {
		s.updates++
	}
	s.failures++
	return agent.Event{}, agent.ErrStorage
}

type streamingProvider struct{ *loopProvider }

func (p streamingProvider) Generate(ctx context.Context, r llm.GenerationRequest, sink func(llm.GenerationEvent)) (llm.GenerationResult, error) {
	for i := 0; i < 1000; i++ {
		sink(llm.GenerationEvent{Kind: "text_delta", Text: "x"})
	}
	return p.loopProvider.Generate(ctx, r, sink)
}
func (p streamingProvider) Continue(ctx context.Context, r llm.GenerationRequest, sink func(llm.GenerationEvent)) (llm.GenerationResult, error) {
	return p.Generate(ctx, r, sink)
}
func TestStreamPreviewAvoidsDurableWriterAndReportsTraceLoss(t *testing.T) {
	s, _, _, a, p := loopFixture(t)
	p.respond = func(llm.GenerationRequest) (llm.GenerationResult, error) {
		return llm.GenerationResult{Text: "complete answer"}, nil
	}
	spy := &traceSpy{ActorStore: a.Store}
	a.Store = spy
	a.Provider = streamingProvider{p}
	var previews, diagnostics atomic.Int64
	a.Sink = func(e agent.Event) {
		if e.Kind == "message_update" {
			previews.Add(1)
		}
	}
	a.TraceError = func(error) { diagnostics.Add(1) }
	mustSubmit(t, a, "stream", "hi")
	mustDrain(t, a)
	if spy.updates != 0 || previews.Load() != 1000 {
		t.Fatal("stream wrote state or lost previews", spy.updates, previews.Load())
	}
	if diagnostics.Load() == 0 || int(diagnostics.Load()) != spy.failures {
		t.Fatal("trace loss silently discarded")
	}
	if scalar(t, s, `SELECT count(*) FROM model_generation WHERE status='completed'`) != 1 || scalar(t, s, `SELECT count(*) FROM run WHERE status='completed'`) != 1 {
		t.Fatal("trace failure changed business outcome")
	}
}

func TestStateErrorKeepsPrivateCauseAndStableClassification(t *testing.T) {
	_, r, _, _, _ := loopFixture(t)
	var err error
	// A direct failing transaction isolates adapter classification from validation.
	err = r.write(context.Background(), func(tx *sql.Tx) error { _, e := tx.Exec(`INSERT INTO missing_private_table VALUES(1)`); return e })
	var operation *agent.OperationError
	if !errors.Is(err, agent.ErrStorage) || !errors.As(err, &operation) || operation.Cause == nil {
		t.Fatal("storage cause lost", err)
	}
	if strings.Contains(err.Error(), "missing_private_table") {
		t.Fatal("private SQL exposed", err)
	}
}

func TestArtifactPublicationRechecksLivenessAfterVerification(t *testing.T) {
	s, _, _, a, _ := loopFixture(t)
	receipt := mustSubmit(t, a, "publish-recheck", "save outputs")
	scope := artifact.Scope{UserID: artifact.LocalUser, AgentID: idAgent(t, s), RunID: receipt.RunID}
	ref := putArtifact(t, s, scope, "verified output")
	proof, err := s.Artifacts().PreparePublication(context.Background(), scope, []string{ref})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Artifacts().Delete(context.Background(), artifactOwner(t, s), ref, 1); err != nil {
		t.Fatal(err)
	}
	err = s.Transaction(context.Background(), func(tx *sql.Tx) error { return s.Artifacts().PublishTx(context.Background(), tx, scope, proof) })
	if !errors.Is(err, artifact.ErrNotFound) || scalar(t, s, `SELECT count(*) FROM artifact_publication`) != 0 {
		t.Fatal("preflight bypassed commit liveness", err)
	}
}
