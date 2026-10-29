package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/z-chenhao/collie/internal/llm"
	"github.com/z-chenhao/collie/internal/llm/account"
	"github.com/z-chenhao/collie/internal/llm/bridge"
)

// Pause after the observer's Guard, before the real usage transaction. A second
// Host/DB handle then mutates auth while Go -> Node -> Pi has already completed.
func TestAccountFinalAcceptanceSerializesWithAuthMutation(t *testing.T) {
	for _, mode := range []string{"unchanged", "refresh", "logout", "reauthorize", "auth-loss", "storage-failure"} {
		t.Run(mode, func(t *testing.T) {
			s, m, peer, secrets, a := accountManager(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			p := activateAccount(t, s, m, a)
			var snapshot llm.Snapshot
			if err := s.Transaction(ctx, func(tx *sql.Tx) error {
				var err error
				snapshot, err = s.Models().FreezeProfile(ctx, tx, p.ProfileID, p.Version)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			other, err := Open(ctx, filepath.Dir(s.path))
			if err != nil {
				t.Fatal(err)
			}
			defer other.Close()
			otherHost := &account.Manager{Store: other.Models(), Secrets: secrets, Provider: peer, LockRoot: m.LockRoot}
			entered, release := make(chan struct{}), make(chan struct{}, 1)
			defer close(release)
			observer := m.Observer(func(ctx context.Context, r llm.GenerationRequest, result llm.GenerationResult, latency time.Duration, cause error) error {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return llm.ErrStorage
				}
				return s.Models().RecordUsage(ctx, r, result, latency, cause)
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
				result, err := client.Generate(ctx, r, func(e llm.GenerationEvent) {
					if e.Kind == "completed" {
						completed <- struct{}{}
					}
				})
				done <- outcome{result, err}
			}()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("generation never reached final persistence")
			}
			if len(completed) != 0 {
				t.Error("completion published before Host acceptance")
			}
			wantErr := error(nil)
			switch mode {
			case "refresh":
				expireAccount(t, secrets, a)
				_, _, err = otherHost.Ensure(ctx, a.ConnectionID)
			case "logout", "reauthorize":
				_, err = otherHost.Logout(ctx, a.ConnectionID)
				if err == nil && mode == "reauthorize" {
					_, err = otherHost.Login(ctx, a.ConnectionID, nil)
				}
				wantErr = llm.AuthRequired
			case "auth-loss":
				err = otherHost.Lose(ctx, a.ConnectionID)
				if !errors.Is(err, llm.AuthRequired) {
					t.Fatal("auth-loss operation did not report closed admission", err)
				}
				err = nil
				wantErr = llm.AuthRequired
			case "storage-failure":
				_, err = other.db.Exec(`CREATE TRIGGER fail_usage BEFORE INSERT ON provider_usage BEGIN SELECT RAISE(ABORT,'unsafe storage detail'); END`)
				wantErr = llm.ErrStorage
			}
			if err != nil {
				t.Fatal(err)
			}
			release <- struct{}{}
			var got outcome
			select {
			case got = <-done:
			case <-ctx.Done():
				t.Fatal("final acceptance did not settle")
			}
			if got.err != wantErr {
				t.Fatalf("error=%v want=%v", got.err, wantErr)
			}
			if wantErr == nil {
				if got.result.Text != "late result" || got.result.Usage == nil || len(completed) != 1 {
					t.Fatal("valid result lost", got.result)
				}
			} else if got.result.Text != "" || got.result.Usage != nil || len(got.result.ToolCalls) != 0 || len(completed) != 0 {
				t.Fatal("rejected result escaped", got.result)
			}
			var code string
			var input, output sql.NullInt64
			err = other.db.QueryRow(`SELECT error_code,input_tokens,output_tokens FROM provider_usage WHERE generation_id=?`, r.GenerationID).Scan(&code, &input, &output)
			if mode == "storage-failure" {
				if err != sql.ErrNoRows {
					t.Fatal("failed transaction left usage", err)
				}
			} else if err != nil {
				t.Fatal(err)
			} else if wantErr != nil {
				if code != llm.AuthRequired.Error() || input.Valid || output.Valid {
					t.Fatal("stale success recorded", code, input, output)
				}
			} else if code != "" || !input.Valid || input.Int64 != 1 || !output.Valid || output.Int64 != 1 {
				t.Fatal("success usage lost", code, input, output)
			}
		})
	}
}

func TestAccountFinalAcceptanceRequiresHostEpoch(t *testing.T) {
	s, m, _, _, a := accountManager(t)
	ctx := context.Background()
	p := activateAccount(t, s, m, a)
	var snapshot llm.Snapshot
	if err := s.Transaction(ctx, func(tx *sql.Tx) error {
		var err error
		snapshot, err = s.Models().FreezeProfile(ctx, tx, p.ProfileID, p.Version)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r := llm.GenerationRequest{GenerationID: uuid.NewString(), RunID: "run", Snapshot: snapshot}
	if err := s.Models().RecordUsage(ctx, r, llm.GenerationResult{Text: "unfenced success"}, time.Second, nil); err != llm.AuthRequired {
		t.Fatal("accepted missing Host epoch", err)
	}
	// Neither the Bridge nor a serialized request can supply the Host fence.
	r.AuthEpoch = 99
	r.Finalize = func(func(bool) error) error { return llm.InvalidProviderResponse }
	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var decoded llm.GenerationRequest
	if err := json.Unmarshal(encoded, &decoded); err != nil || decoded.AuthEpoch != 0 || decoded.Finalize != nil {
		t.Fatal("Host acceptance state crossed Bridge protocol", err)
	}
}
