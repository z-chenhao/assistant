package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/computer"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/tool"
	"github.com/z-chenhao/erii/internal/tool/codemode"
)

type lifecycleFake struct {
	ensures, restores, checkpoints, destroys int
	instance                                 computer.Instance
	corrupt                                  bool
}

func (f *lifecycleFake) Ensure(ctx context.Context, r computer.Request) (computer.Instance, error) {
	if err := r.Fence(ctx); err != nil {
		return computer.Instance{}, err
	}
	f.ensures++
	f.instance = r.Instance
	return r.Instance, nil
}
func (f *lifecycleFake) Restore(ctx context.Context, r computer.Request, cp computer.Checkpoint) (computer.Instance, error) {
	if err := r.Fence(ctx); err != nil {
		return computer.Instance{}, err
	}
	if cp.ComputerID != r.Instance.ComputerID || cp.SourceEpoch >= r.Instance.Epoch {
		return computer.Instance{}, computer.ErrStale
	}
	f.restores++
	f.instance = r.Instance
	return r.Instance, nil
}
func (f *lifecycleFake) Checkpoint(ctx context.Context, r computer.Request) (computer.Checkpoint, error) {
	if err := r.Fence(ctx); err != nil {
		return computer.Checkpoint{}, err
	}
	f.checkpoints++
	hash, err := computer.Fingerprint(r.Manifest)
	if err != nil {
		return computer.Checkpoint{}, err
	}
	cp := computer.Checkpoint{ID: r.CheckpointID, ComputerID: r.Instance.ComputerID, SourceEpoch: r.Instance.Epoch, ManifestSHA256: hash}
	if f.corrupt {
		cp.ComputerID = "wrong-computer"
	}
	return cp, nil
}
func (f *lifecycleFake) Destroy(ctx context.Context, r computer.Request) error {
	if err := r.Fence(ctx); err != nil {
		return err
	}
	f.destroys++
	return nil
}

func computerClaims(t *testing.T) (*Store, *Runs, []*codeBridge, []tool.Invocation) {
	t.Helper()
	ctx := context.Background()
	defs := append(computer.Definitions("pinned-test"), codemode.Definition())
	for _, spec := range agent.ProviderTools([]tool.Spec{defs[0].Spec, defs[1].Spec, defs[2].Spec, defs[3].Spec}) {
		if spec.Name != codemode.ToolName {
			t.Fatal("internal bridge exposed to model", spec.Name)
		}
	}
	calls := []llm.ToolCall{{ID: "first", Name: codemode.ToolName, Arguments: `{"source":"return 1;"}`}, {ID: "second", Name: codemode.ToolName, Arguments: `{"source":"return 1;"}`}}
	s, r, _, e, exchange := admitPolicy(t, defs, calls)
	invs, err := e.Store.Accept(ctx, exchange, calls, e.Registry.Describe())
	if err != nil {
		t.Fatal(err)
	}
	var bridges []*codeBridge
	for _, inv := range invs {
		if err := e.Store.Start(ctx, inv.ID); err != nil {
			t.Fatal(err)
		}
		b := &codeBridge{t: e.Store.(*tools), registry: e.Registry, attempt: newID(), req: codemode.ExecutionRequest{ProtocolVersion: codemode.ProtocolVersion, RuntimeVersion: codemode.RuntimeVersion, PreludeVersion: codemode.PreludeVersion, ExecutionID: inv.ID, ParentInvocationID: inv.ID, RunID: e.Store.(*tools).claim.RunID(), Source: "return 1;", SourceHash: digest("return 1;"), Limits: codemode.DefaultLimits()}}
		if err := b.claim(ctx, inv); err != nil {
			t.Fatal(err)
		}
		bridges = append(bridges, b)
	}
	return s, r, bridges, invs
}

func TestComputerIdentityRecoveryArbitrationAndFencing(t *testing.T) {
	s, r, bridges, invs := computerClaims(t)
	ctx := context.Background()
	provider := &lifecycleFake{}
	r.ConfigureComputer(provider, computer.Compatibility{ProviderVersion: "fixture"}, computer.RestorationManifest{Version: 1, ArtifactRefs: []string{"artifact:stable-fixture"}})
	first, err := bridges[0].computer().Ensure(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE computer SET phase='checkpointed',execution_id=NULL,version=version+1`,
		`UPDATE computer SET instance_epoch=instance_epoch+1,version=version+1`,
		`UPDATE computer SET compatibility_json='{}',version=version+1`,
		`UPDATE computer SET manifest_json='{}',version=version+1`,
		`UPDATE computer SET version=version+2`,
	} {
		if _, err := s.db.Exec(statement); err == nil {
			t.Fatal("illegal state accepted", statement)
		}
	}
	if _, err := bridges[1].computer().Ensure(ctx); !errors.Is(err, computer.ErrBusy) {
		t.Fatal("live owner stolen", err)
	}
	if err := bridges[1].computer().Idle(ctx); err != nil || provider.checkpoints != 0 {
		t.Fatal("unrelated cell reclaimed Computer", err)
	}
	if err := bridges[0].computer().Idle(ctx); err != nil {
		t.Fatal(err)
	}
	if scalar(t, s, `SELECT count(*) FROM computer WHERE phase='checkpointed' AND execution_id IS NULL`) != 1 || provider.checkpoints != 1 || provider.destroys != 1 {
		t.Fatal("idle did not checkpoint/release")
	}
	second, err := bridges[1].computer().Ensure(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.ComputerID != second.ComputerID || second.Epoch != first.Epoch+1 || first.NativeName == second.NativeName || provider.restores != 1 {
		t.Fatal(first, second, provider)
	}
	if err := (computerStore{bridges[0]}).Fence(ctx); !errors.Is(err, computer.ErrStale) {
		t.Fatal("late old instance accepted", err)
	}
	if _, err := bridges[0].computer().Ensure(ctx); !errors.Is(err, computer.ErrStale) {
		t.Fatal("retired instance returned as live", err)
	}
	// Simulate Host death and recovery of the same accepted invocation. The
	// old VM is checkpointed before its replacement gets a new epoch.
	if _, err := s.db.Exec(`UPDATE code_execution_state SET claim_expires_at=0,version=version+1 WHERE execution_id=?`, bridges[1].req.ExecutionID); err != nil {
		t.Fatal(err)
	}
	recovered := &codeBridge{t: bridges[1].t, registry: bridges[1].registry, attempt: newID(), req: codemode.ExecutionRequest{ProtocolVersion: codemode.ProtocolVersion, RuntimeVersion: codemode.RuntimeVersion, PreludeVersion: codemode.PreludeVersion, ExecutionID: invs[1].ID, ParentInvocationID: invs[1].ID, RunID: bridges[1].req.RunID, Source: "return 1;", SourceHash: digest("return 1;"), Limits: codemode.DefaultLimits()}}
	if err := recovered.claim(ctx, invs[1]); err != nil {
		t.Fatal(err)
	}
	third, err := recovered.computer().Ensure(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if third.ComputerID != first.ComputerID || third.Epoch != 3 || provider.checkpoints != 2 || provider.restores != 2 {
		t.Fatal(third, provider)
	}
	if err := (computerStore{bridges[1]}).Fence(ctx); !errors.Is(err, agent.ErrClaimLost) {
		t.Fatal("old Host result accepted", err)
	}
	if err := recovered.computer().Idle(ctx); err != nil {
		t.Fatal(err)
	}
	if scalar(t, s, `SELECT count(*) FROM computer_operation WHERE status='accepted'`) != 0 {
		t.Fatal("unfinished operation")
	}
}

func TestComputerCheckpointRejectsWrongProviderIdentity(t *testing.T) {
	s, r, b, _ := computerClaims(t)
	ctx := context.Background()
	f := &lifecycleFake{corrupt: true}
	r.ConfigureComputer(f, computer.Compatibility{}, computer.RestorationManifest{Version: 1})
	if _, err := b[0].computer().Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b[0].computer().Idle(ctx); !errors.Is(err, computer.ErrStale) && !errors.Is(err, computer.ErrIntegrity) {
		t.Fatal(err)
	}
	if f.destroys != 0 || scalar(t, s, `SELECT count(*) FROM computer WHERE checkpoint_json IS NOT NULL`) != 0 {
		t.Fatal("bad checkpoint committed")
	}
}

func TestComputerCannotBypassAcceptedCodeBoundary(t *testing.T) {
	_, r, b, _ := computerClaims(t)
	ctx := context.Background()
	f := &lifecycleFake{}
	r.ConfigureComputer(f, computer.Compatibility{}, computer.RestorationManifest{Version: 1})
	result, err := b[0].t.executeComputer(ctx, tool.Invocation{Call: llm.ToolCall{Name: "builtin__computer__discover", Arguments: `{"method":"getState"}`}})
	if err != nil || result.Error == nil || result.Error.Code != "PERMISSION_DENIED" || f.ensures != 0 {
		t.Fatal(result, err)
	}
	// Request drift under the frozen lifecycle operation fails closed.
	if _, err := b[0].computer().Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	r.computerManifest.ArtifactRefs = []string{"artifact:changed"}
	if _, err := b[0].computer().Ensure(ctx); !errors.Is(err, computer.ErrCompatibility) {
		t.Fatal("manifest drift accepted", err)
	}
}

func TestComputerMigrationFromPopulatedV20PreservesAcceptedWork(t *testing.T) {
	s, _, b, _ := computerClaims(t)
	ctx := context.Background()
	path := s.path
	var request string
	if err := s.db.QueryRow(`SELECT request_json FROM model_generation LIMIT 1`).Scan(&request); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DROP TABLE computer_operation; DROP TABLE computer; PRAGMA user_version=20`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var after string
	if err := reopened.db.QueryRow(`SELECT request_json FROM model_generation LIMIT 1`).Scan(&after); err != nil || after != request {
		t.Fatal("accepted request changed", err)
	}
	if scalar(t, reopened, `PRAGMA user_version`) != 21 || scalar(t, reopened, `SELECT count(*) FROM code_execution_state WHERE execution_id=? AND claim_token=?`, b[0].req.ExecutionID, b[0].req.ClaimToken) != 1 {
		t.Fatal("migration changed owner")
	}
	rows, err := reopened.db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("broken foreign key")
	}
	// A failed migration leaves both version and existing records intact.
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "prior.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrate(ctx, db, migrations[:20]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE computer_operation(conflict TEXT)`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, db, migrations); err == nil {
		t.Fatal("conflicting migration accepted")
	}
	var version, count int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name='computer'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if version != 20 || count != 0 {
		t.Fatal("partially committed migration", version, count)
	}
}
