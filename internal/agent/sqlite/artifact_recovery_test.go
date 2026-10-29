package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/z-chenhao/collie/internal/agent"
	"github.com/z-chenhao/collie/internal/artifact"
	"github.com/z-chenhao/collie/internal/llm"
	"github.com/z-chenhao/collie/internal/tool"
)

type receiptReader struct {
	io.Reader
	ctx    context.Context
	closed atomic.Int32
}

func (r *receiptReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}
func (r *receiptReader) Close() error { r.closed.Add(1); return nil }

// recoveryFixture leaves a real approved effect at either crash boundary. A
// reclaimed Run owns running recovery; terminal unknown reconciliation needs
// no claim and must never revive the Run.
func recoveryFixture(t *testing.T, path string, query func(context.Context, tool.Request) tool.Evidence) (*Store, *Runs, tool.Executor, string, []llm.ToolCall, *atomic.Int32) {
	t.Helper()
	ctx := context.Background()
	invoked := &atomic.Int32{}
	def := tool.Definition{Spec: tool.Spec{Name: "mcp__fixture__receipt", Version: 1, InputSchema: `{"type":"object","additionalProperties":false}`, Permission: "external_effect", ReplayPolicy: "query_before_retry", Risk: "high", TimeoutMS: 100, OutputBytes: 1024, Provider: "fixture", ConflictPolicy: "none"},
		Invoke: func(context.Context, tool.Request) tool.Outcome { invoked.Add(1); return tool.Outcome{Ambiguous: true} }, Query: query}
	calls := []llm.ToolCall{{ID: "receipt", Name: def.Spec.Name, Arguments: `{}`}}
	s, r, c, e, x := admitPolicy(t, []tool.Definition{def}, calls)
	if _, err := e.Execute(ctx, x, calls); !errors.Is(err, tool.ErrSuspended) {
		t.Fatal(err)
	}
	approvePolicy(t, s, r)
	if path == "running" {
		invs, err := e.Store.Accept(ctx, x, calls, e.Registry.Describe())
		if err != nil {
			t.Fatal(err)
		}
		if err = e.Store.Start(ctx, invs[0].ID); err != nil {
			t.Fatal(err)
		}
		def.Invoke(ctx, tool.Request{Arguments: invs[0].Call.Arguments, IdempotencyKey: invs[0].Key})
	} else {
		result, err := e.Execute(ctx, x, calls)
		if err != nil || result[0].Error == nil || result[0].Error.Code != "UNKNOWN_SIDE_EFFECT" {
			t.Fatal(result, err)
		}
	}
	if path == "ended_unknown" {
		run, err := r.Load(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		if err = r.Fail(ctx, c, run.Version, "UNKNOWN_SIDE_EFFECT"); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := r.Release(ctx, c); err != nil {
			t.Fatal(err)
		}
		c, err := r.Claim(ctx, c.RunID())
		if err != nil {
			t.Fatal(err)
		}
		e.Store = r.ToolStore(c)
	}
	return s, r, e, x, calls, invoked
}

func TestArtifactQueryRecoveryCommitsExactReceipt(t *testing.T) {
	for _, path := range []string{"running", "unknown", "ended_unknown"} {
		t.Run(path, func(t *testing.T) {
			// Keep the frozen 100ms budget, but do not spend it on host
			// scheduling/filesystem latency in this successful receipt contract.
			synctest.Test(t, func(t *testing.T) {
				ctx := context.Background()
				payload := strings.Repeat("\x00authoritative receipt\xff", 4096)
				var reader *receiptReader
				var queries int
				s, r, e, x, calls, invoked := recoveryFixture(t, path, func(ctx context.Context, _ tool.Request) tool.Evidence {
					queries++
					reader = &receiptReader{Reader: strings.NewReader(payload), ctx: ctx}
					return tool.Evidence{State: "succeeded", Result: tool.Result{Output: &tool.BinaryOutput{Name: "receipt.bin", MediaType: "application/octet-stream", Reader: reader}}}
				})
				runVersion := scalar(t, s, `SELECT version FROM run LIMIT 1`)
				var returned tool.Result
				if path != "running" {
					if err := r.ReconcileUnknown(ctx); err != nil {
						t.Fatal(err)
					}
					observations, err := r.ToolOutcomes(ctx)
					if err != nil || len(observations) != 1 {
						t.Fatal(observations, err)
					}
					returned = observations[0].Result
				}
				if path != "ended_unknown" {
					results, err := e.Execute(ctx, x, calls)
					if err != nil || len(results) != 1 {
						t.Fatal(results, err)
					}
					if path == "unknown" && !reflect.DeepEqual(returned, results[0]) {
						t.Fatal("control-plane/caller differ", returned, results)
					}
					returned = results[0]
				}
				if returned.Error != nil || returned.Output != nil || len(returned.ArtifactRefs) != 1 {
					t.Fatal(returned)
				}
				var persisted tool.Result
				var raw, proof, ref string
				if err := s.db.QueryRow(`SELECT result_json,reconciliation_json,result_ref FROM tool_invocation WHERE status='succeeded'`).Scan(&raw, &proof, &ref); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal([]byte(raw), &persisted); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(returned, persisted) || proof != raw || ref != returned.ArtifactRefs[0] {
					t.Fatal("caller/committed/proof differ", returned, raw, proof, ref)
				}
				if got := readArtifact(t, s, artifactOwner(t, s), ref); got != payload {
					t.Fatal("receipt bytes lost")
				}
				d, err := s.Artifacts().Describe(ctx, artifactOwner(t, s), ref)
				hash := sha256.Sum256([]byte(payload))
				if err != nil || d.ContentHash != hex.EncodeToString(hash[:]) || d.ByteSize != int64(len(payload)) {
					t.Fatal(d, err)
				}
				var runID string
				if err := s.db.QueryRow(`SELECT run_id FROM tool_invocation`).Scan(&runID); err != nil {
					t.Fatal(err)
				}
				scope := artifact.Scope{UserID: artifact.LocalUser, AgentID: idAgent(t, s), RunID: runID}
				for _, bad := range []artifact.Scope{{UserID: "other", AgentID: scope.AgentID, RunID: scope.RunID}, {UserID: scope.UserID, AgentID: "other", RunID: scope.RunID}, {UserID: scope.UserID, AgentID: scope.AgentID, RunID: "other"}} {
					if _, err := s.Artifacts().Open(ctx, bad, ref); !errors.Is(err, artifact.ErrNotFound) {
						t.Fatal("scope leak", bad, err)
					}
				}
				if readArtifact(t, s, scope, ref) != payload {
					t.Fatal("Run cannot read own receipt")
				}
				if path != "ended_unknown" {
					replay, err := e.Execute(ctx, x, calls)
					if err != nil || !reflect.DeepEqual(replay, []tool.Result{returned}) {
						t.Fatal("replay changed", replay, err)
					}
				}
				if err := r.ReconcileUnknown(ctx); err != nil {
					t.Fatal(err)
				}
				if reader.closed.Load() != 1 || queries != 1 || invoked.Load() != 1 || scalar(t, s, `SELECT attempt_count FROM tool_invocation`) != 1 || scalar(t, s, `SELECT count(*) FROM artifact`) != 1 || scalar(t, s, `SELECT count(*) FROM artifact_reference WHERE holder_kind='tool'`) != 1 || scalar(t, s, `SELECT version FROM run LIMIT 1`) != runVersion {
					t.Fatal("recovery leaked, revived or repeated", reader.closed.Load(), queries, invoked.Load())
				}
			})
		})
	}
}

// The exact-receipt contract above assumes its query/receipt fits the frozen
// budget. Expiry is a different legal outcome: retain unknown, then permit a
// fresh evidence query without ever redispatching the external effect.
func TestArtifactQueryRecoveryFrozenDeadline(t *testing.T) {
	for _, path := range []string{"running", "unknown", "ended_unknown"} {
		t.Run(path, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx := context.Background()
				payload := "authoritative binary receipt"
				queries := 0
				var readers []*receiptReader
				s, r, e, x, calls, invoked := recoveryFixture(t, path, func(ctx context.Context, _ tool.Request) tool.Evidence {
					queries++
					deadline, ok := ctx.Deadline()
					if !ok || time.Until(deadline) != 100*time.Millisecond {
						t.Fatal("query did not retain its frozen 100ms deadline")
					}
					if queries == 1 {
						time.Sleep(100 * time.Millisecond)
						<-ctx.Done()
					}
					reader := &receiptReader{Reader: strings.NewReader(payload), ctx: ctx}
					readers = append(readers, reader)
					return tool.Evidence{State: "succeeded", Result: tool.Result{Output: &tool.BinaryOutput{Name: "receipt.bin", MediaType: "application/octet-stream", Reader: reader}}}
				})
				runVersion := scalar(t, s, `SELECT version FROM run LIMIT 1`)
				recover := func() tool.Result {
					if path == "running" {
						results, err := e.Execute(ctx, x, calls)
						if err != nil || len(results) != 1 {
							t.Fatal(results, err)
						}
						return results[0]
					}
					if err := r.ReconcileUnknown(ctx); err != nil {
						t.Fatal(err)
					}
					observations, err := r.ToolOutcomes(ctx)
					if err != nil || len(observations) != 1 {
						t.Fatal(observations, err)
					}
					return observations[0].Result
				}
				first := recover()
				if first.Error == nil || first.Error.Code != "UNKNOWN_SIDE_EFFECT" || scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE status='unknown'`) != 1 || scalar(t, s, `SELECT count(*) FROM artifact`) != 0 || readers[0].closed.Load() != 1 {
					t.Fatal("expired query settled a receipt or leaked its stream", first)
				}
				second := recover()
				if second.Error != nil || len(second.ArtifactRefs) != 1 || readArtifact(t, s, artifactOwner(t, s), second.ArtifactRefs[0]) != payload {
					t.Fatal("fresh authoritative query did not retain its receipt", second)
				}
				if queries != 2 || invoked.Load() != 1 || readers[1].closed.Load() != 1 || scalar(t, s, `SELECT attempt_count FROM tool_invocation`) != 1 || scalar(t, s, `SELECT count(*) FROM artifact`) != 1 || scalar(t, s, `SELECT version FROM run LIMIT 1`) != runVersion {
					t.Fatal("query retry repeated an effect, revived a Run or leaked a stream")
				}
			})
		})
	}
}

func TestArtifactQueryRecoveryUnsafeReceiptStaysUnknown(t *testing.T) {
	for _, path := range []string{"running", "unknown"} {
		for _, kind := range []string{"deadline", "cancel", "read_error", "forged_ref", "invalid_metadata", "commit_failure", "rejected_evidence"} {
			t.Run(path+"/"+kind, func(t *testing.T) {
				var closeCount *atomic.Int32
				blocked := &blockedArtifactReader{closed: make(chan struct{}), started: make(chan struct{})}
				s, r, e, x, calls, invoked := recoveryFixture(t, path, func(ctx context.Context, _ tool.Request) tool.Evidence {
					result := tool.Result{}
					if kind == "deadline" || kind == "cancel" {
						result.Output = &tool.BinaryOutput{Name: "receipt.bin", MediaType: "application/octet-stream", Reader: blocked}
					} else {
						reader := &receiptReader{Reader: strings.NewReader("receipt"), ctx: ctx}
						closeCount = &reader.closed
						result.Output = &tool.BinaryOutput{Name: "receipt.bin", MediaType: "application/octet-stream", Reader: reader}
						switch kind {
						case "read_error":
							reader.Reader = errorReceiptReader{}
						case "forged_ref":
							result.ArtifactRefs = []string{"artifact:" + strings.Repeat("0", 64)}
						case "invalid_metadata":
							result.Output.Name = "../receipt"
						}
					}
					state := "succeeded"
					if kind == "rejected_evidence" {
						state = "unknown"
					}
					return tool.Evidence{State: state, Result: result}
				})
				if kind == "commit_failure" {
					if _, err := s.db.Exec(`CREATE TRIGGER reject_recovered_receipt BEFORE UPDATE ON tool_invocation WHEN NEW.reconciliation_json IS NOT NULL BEGIN SELECT RAISE(ABORT,'injected commit failure'); END;`); err != nil {
						t.Fatal(err)
					}
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan error, 1)
				go func() {
					if path == "unknown" {
						done <- r.ReconcileUnknown(ctx)
					} else {
						_, err := e.Execute(ctx, x, calls)
						done <- err
					}
				}()
				if kind == "cancel" {
					select {
					case <-blocked.started:
						cancel()
					case err := <-done:
						t.Fatal("stream never consumed", err)
					case <-time.After(time.Second):
						cancel()
						blocked.Close()
						<-done
						t.Fatal("stream admission stalled")
					}
				}
				select {
				case err := <-done:
					if err != nil && !(errors.Is(err, agent.ErrStorage) && (kind == "read_error" || kind == "commit_failure")) {
						t.Fatal(err)
					}
				case <-time.After(2 * time.Second):
					cancel()
					blocked.Close()
					<-done
					t.Fatal("receipt recovery failed to finish")
				}
				if kind == "deadline" || kind == "cancel" {
					select {
					case <-blocked.closed:
					default:
						t.Fatal("stream not closed")
					}
				}

				if closeCount != nil && closeCount.Load() != 1 {
					t.Fatal("receipt stream leaked", closeCount.Load())
				}
				if invoked.Load() != 1 || scalar(t, s, `SELECT count(*) FROM tool_invocation WHERE status='unknown'`) != 1 || scalar(t, s, `SELECT count(*) FROM artifact`) != 0 || scalar(t, s, `SELECT count(*) FROM artifact_reference WHERE holder_kind='tool'`) != 0 || scalar(t, s, `SELECT count(*) FROM artifact_publication`) != 0 {
					t.Fatal("unusable receipt settled or repeated the effect")
				}
				// Query may be repeated, but it must never dispatch an effect.
				if _, err := e.Execute(context.Background(), x, calls); err != nil && !errors.Is(err, agent.ErrStorage) {
					t.Fatal(err)
				}
				if invoked.Load() != 1 {
					t.Fatal("effect redispatched")
				}
			})
		}
	}
}

type errorReceiptReader struct{}

func (errorReceiptReader) Read([]byte) (int, error) { return 0, errors.New("receipt interrupted") }
