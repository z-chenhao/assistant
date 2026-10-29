package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/z-chenhao/collie/internal/llm"
	"github.com/z-chenhao/collie/internal/llm/bridge"
)

// The two pre-commit barriers distinguish observer delay from the real usage
// transaction. The post-commit barrier proves a late ack preserves acceptance.
func TestAccountCancellationAtFinalAcceptance(t *testing.T) {
	for _, operation := range []string{"generate", "continue"} {
		for _, action := range []string{"cancel", "dispose", "context"} {
			for _, phase := range []string{"observer", "transaction", "committed"} {
				t.Run(operation+"/"+action+"/"+phase, func(t *testing.T) {
					s, m, _, _, a := accountManager(t)
					ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
					defer stop()
					generationCtx, cancelGeneration := context.WithCancel(ctx)
					defer cancelGeneration()
					p := activateAccount(t, s, m, a)
					var snapshot llm.Snapshot
					if err := s.Transaction(ctx, func(tx *sql.Tx) error {
						var err error
						snapshot, err = s.Models().FreezeProfile(ctx, tx, p.ProfileID, p.Version)
						return err
					}); err != nil {
						t.Fatal(err)
					}
					entered, release := make(chan struct{}), make(chan struct{}, 1)
					defer close(release)
					barrier := func() {
						close(entered)
						select {
						case <-release:
						case <-ctx.Done():
						}
					}
					observer := m.Observer(func(ctx context.Context, r llm.GenerationRequest, result llm.GenerationResult, latency time.Duration, cause error) error {
						if phase == "observer" {
							barrier()
						} else if phase == "transaction" {
							finalize := r.Finalize
							if finalize == nil {
								return llm.InvalidProviderResponse
							}
							r.Finalize = func(commit func(bool) error) error {
								barrier() // BEGIN IMMEDIATE and auth check already ran
								return finalize(commit)
							}
						}
						err := s.Models().RecordUsage(ctx, r, result, latency, cause)
						if phase == "committed" {
							barrier() // real INSERT/COMMIT finished; publication has not
						}
						return err
					})
					script, err := filepath.Abs("../../../provider-bridge/test/account-completion.mjs")
					if err != nil {
						t.Fatal(err)
					}
					client, err := bridge.OpenLocal(ctx, "node", script, m.Resolver(nil), observer)
					if err != nil {
						t.Fatal(err)
					}
					defer client.Close()
					type outcome struct {
						result llm.GenerationResult
						err    error
					}
					done := make(chan outcome, 1)
					completed := make(chan struct{}, 1)
					run := uuid.NewString()
					r := llm.GenerationRequest{GenerationID: uuid.NewString(), RunID: run, SessionID: run, Snapshot: snapshot, Messages: []llm.Message{{Role: "user", Text: "Reply OK"}}}
					go func() {
						sink := func(e llm.GenerationEvent) {
							if e.Kind == "completed" {
								completed <- struct{}{}
							}
						}
						generate := client.Generate
						if operation == "continue" {
							generate = client.Continue
						}
						result, err := generate(generationCtx, r, sink)
						done <- outcome{result, err}
					}()
					select {
					case <-entered:
					case <-ctx.Done():
						t.Fatal("acceptance barrier not reached")
					}
					if len(completed) != 0 {
						t.Fatal("completion escaped before barrier")
					}
					switch action {
					case "cancel":
						err = client.Cancel(ctx, r.GenerationID)
					case "dispose":
						err = client.Dispose(ctx, run)
					case "context":
						cancelGeneration()
					}
					if err != nil {
						t.Fatal("acknowledgement blocked by pending observer/transaction", err)
					}
					release <- struct{}{}
					var got outcome
					select {
					case got = <-done:
					case <-ctx.Done():
						t.Fatal("generation did not settle")
					}
					var code string
					var input, output sql.NullInt64
					if err := s.db.QueryRow(`SELECT error_code,input_tokens,output_tokens FROM provider_usage WHERE generation_id=?`, r.GenerationID).Scan(&code, &input, &output); err != nil {
						t.Fatal(err)
					}
					if phase == "committed" {
						if got.err != nil || got.result.Text != "late result" || got.result.Usage == nil || len(completed) != 1 || code != "" || !input.Valid || !output.Valid {
							t.Fatal("late cancellation changed committed result", got, code, input, output)
						}
					} else if got.err != llm.GenerationCancelled || got.result.Text != "" || got.result.Usage != nil || len(got.result.ToolCalls) != 0 || len(completed) != 0 || code != llm.GenerationCancelled.Error() || input.Valid || output.Valid {
						t.Fatal("pre-commit cancellation admitted success", got, code, input, output)
					}
					t.Logf("operation=%s action=%s phase=%s error=%v completed=%d stored_error=%q", operation, action, phase, got.err, len(completed), code)
				})
			}
		}
	}
}
