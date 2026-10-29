// Package sqlite persists Computer lifecycle plans inside the Host's existing
// state transaction. It neither opens a database nor dispatches external work.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"

	"github.com/google/uuid"
	"github.com/z-chenhao/collie/internal/computer"
	"github.com/z-chenhao/collie/internal/llm"
)

// ExecutionHost binds an accepted Code execution. Write must check its live
// fence inside the same transaction before invoking change. RecordEvent must
// enqueue the Run event in that transaction. The adapter cannot bypass either.
type ExecutionHost interface {
	Write(context.Context, func(*sql.Tx) error) error
	RecordEvent(context.Context, *sql.Tx, string, int64) error
}
type Store struct {
	host           ExecutionHost
	executionID    string
	executionEpoch int64
}

func Bind(host ExecutionHost, id string, epoch int64) Store {
	return Store{host: host, executionID: id, executionEpoch: epoch}
}

var _ computer.LifecycleStore = Store{}
var ErrStorage = errors.New("computer lifecycle storage unavailable")

func newID() string { return uuid.Must(uuid.NewV7()).String() }
func encoded(value any) (string, error) {
	raw, err := llm.CanonicalJSON(value)
	return string(raw), err
}
func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func (s Store) Fence(ctx context.Context) error {
	return s.host.Write(ctx, func(tx *sql.Tx) error {
		var valid bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM computer WHERE execution_id=? AND execution_epoch=?)`, s.executionID, s.executionEpoch).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return computer.ErrStale
		}
		return nil
	})
}

func decode(raw string, to any) error {
	if json.Unmarshal([]byte(raw), to) != nil {
		return ErrStorage
	}
	return nil
}

// The previous instance may have been disposed by a cancelled invocation.
// Its immutable idle request remains authoritative even after supersession.
// Derive this link from the retained ledger on every claim, including recovery
// of an ensure operation, without changing either frozen record.
func attachCleanup(ctx context.Context, tx *sql.Tx, plan *computer.Plan) error {
	if plan.Previous == nil || plan.Done {
		return nil
	}
	var raw string
	err := tx.QueryRowContext(ctx, `SELECT request_json FROM computer_operation WHERE kind='idle' AND json_extract(request_json,'$.instance.computer_id')=? AND json_extract(request_json,'$.instance.instance_epoch')=? ORDER BY rowid DESC LIMIT 1`, plan.Previous.ComputerID, plan.Previous.Epoch).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	var request computer.Request
	if err := decode(raw, &request); err != nil {
		return err
	}
	if request.Instance != *plan.Previous || !reflect.DeepEqual(request.Compatibility, plan.Request.Compatibility) || !reflect.DeepEqual(request.Manifest, plan.Request.Manifest) {
		return computer.ErrCompatibility
	}
	plan.Cleanup = &request
	return nil
}

func (s Store) Begin(ctx context.Context, kind string, compatibility computer.Compatibility, manifest computer.RestorationManifest) (computer.Plan, error) {
	var plan computer.Plan
	if kind != "ensure" && kind != "idle" || manifest.Validate() != nil {
		return plan, computer.ErrInvalid
	}
	err := s.host.Write(ctx, func(tx *sql.Tx) error {
		plan = computer.Plan{}
		accepted, found, err := s.acceptedPlan(ctx, tx, kind, compatibility, manifest)
		if err != nil {
			return err
		}
		if found {
			plan = accepted
			return nil
		}
		var previous, checkpoint sql.NullString
		var instance computer.Instance
		var instanceJSON, compatibilityJSON, storedManifest, phase string
		var owner sql.NullString
		var ownerEpoch int64
		var version int64
		err = tx.QueryRowContext(ctx, `SELECT instance_json,compatibility_json,manifest_json,checkpoint_json,phase,execution_id,execution_epoch,version FROM computer WHERE singleton=1`).Scan(&instanceJSON, &compatibilityJSON, &storedManifest, &checkpoint, &phase, &owner, &ownerEpoch, &version)
		fresh := err == sql.ErrNoRows
		if err != nil && !fresh {
			return err
		}
		compatibilityRaw, err := encoded(compatibility)
		if err != nil {
			return err
		}
		manifestRaw, err := encoded(manifest)
		if err != nil {
			return err
		}
		if fresh && kind == "idle" || !fresh && kind == "idle" && (!owner.Valid || owner.String != s.executionID) {
			plan.Empty = true
			return nil
		}
		if fresh {
			instance = computer.Instance{ComputerID: newID(), Epoch: 1}
		} else {
			if compatibilityJSON != compatibilityRaw || storedManifest != manifestRaw {
				return computer.ErrCompatibility
			}
			if err := decode(instanceJSON, &instance); err != nil {
				return err
			}
			if kind == "idle" && phase == "checkpointing" {
				// A new Code claim resumes the same accepted disposal. Its
				// checkpoint may already be committed and its instance deleted;
				// replacing these frozen IDs would require a nonexistent source.
				var priorRequest string
				if err := tx.QueryRowContext(ctx, `SELECT request_json FROM computer_operation WHERE execution_id=? AND execution_epoch=? AND kind='idle' AND status='accepted'`, owner.String, ownerEpoch).Scan(&priorRequest); err != nil {
					return err
				}
				if err := decode(priorRequest, &plan.Request); err != nil {
					return err
				}
			}
			if owner.Valid && owner.String != s.executionID {
				var active bool
				if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tool_invocation i JOIN run r USING(run_id) WHERE i.invocation_id=? AND i.status NOT IN ('succeeded','failed','cancelled','unknown') AND r.status IN ('queued','running','waiting'))`, owner.String).Scan(&active); err != nil {
					return err
				}
				if active || kind == "idle" {
					return computer.ErrBusy
				}
			}
			if kind == "ensure" {
				if owner.Valid && owner.String == s.executionID && ownerEpoch != s.executionEpoch && phase == "ready" {
					var resumable bool
					if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM computer_lease_resume WHERE execution_id=? AND execution_epoch=?)`, owner.String, ownerEpoch).Scan(&resumable); err != nil {
						return err
					}
					if resumable {
						if _, err := tx.ExecContext(ctx, `UPDATE computer SET execution_epoch=?,version=version+1 WHERE singleton=1 AND version=?`, s.executionEpoch, version); err != nil {
							return err
						}
						if _, err := tx.ExecContext(ctx, `DELETE FROM computer_lease_resume WHERE execution_id=?`, owner.String); err != nil {
							return err
						}
						plan.Request.Instance = instance
						plan.Done = true
						return nil
					}
				}
				if owner.Valid && owner.String == s.executionID && ownerEpoch == s.executionEpoch && phase == "ready" {
					plan.Request.Instance = instance
					plan.Done = true
					return nil
				}
				if phase == "restoring" {
					// Reattach an incomplete frozen operation. No second run request
					// is sent if its earlier outcome cannot be proved by the provider.
					if !owner.Valid {
						return computer.ErrStale
					}
					var priorRequest string
					if err := tx.QueryRowContext(ctx, `SELECT request_json,previous_json,checkpoint_json FROM computer_operation WHERE execution_id=? AND execution_epoch=? AND kind='ensure'`, owner.String, ownerEpoch).Scan(&priorRequest, &previous, &checkpoint); err != nil {
						return err
					}
					if err := decode(priorRequest, &plan.Request); err != nil {
						return err
					}
					if previous.Valid {
						if err := decode(previous.String, &plan.Previous); err != nil {
							return err
						}
					}
				} else {
					if owner.Valid {
						old := instance
						plan.Previous = &old
					}
					instance.Epoch++
				}
			}
		}
		if owner.Valid && (owner.String != s.executionID || ownerEpoch != s.executionEpoch) {
			if _, err := tx.ExecContext(ctx, `UPDATE computer_operation SET status='superseded' WHERE execution_id=? AND execution_epoch=? AND status='accepted'`, owner.String, ownerEpoch); err != nil {
				return err
			}
		}
		return s.publishPlan(ctx, tx, &plan, lifecycleChange{compatibility: compatibility, manifest: manifest, kind: kind, instance: instance, compatibilityRaw: compatibilityRaw, manifestRaw: manifestRaw, checkpoint: checkpoint, fresh: fresh, version: version})
	})
	return plan, err
}

func (s Store) mutate(ctx context.Context, plan computer.Plan, action func(*sql.Tx, string, int64) error) error {
	return s.host.Write(ctx, func(tx *sql.Tx) error {
		var version int64
		if err := tx.QueryRowContext(ctx, `SELECT version FROM computer WHERE execution_id=? AND execution_epoch=? AND instance_epoch=?`, s.executionID, s.executionEpoch, plan.Request.Instance.Epoch).Scan(&version); err != nil {
			if err == sql.ErrNoRows {
				return computer.ErrStale
			}
			return err
		}
		requestRaw, err := encoded(plan.Request)
		if err != nil {
			return err
		}
		var kind string
		if err := tx.QueryRowContext(ctx, `SELECT kind FROM computer_operation WHERE execution_id=? AND execution_epoch=? AND request_json=? AND status='accepted'`, s.executionID, s.executionEpoch, requestRaw).Scan(&kind); err != nil {
			return computer.ErrStale
		}
		return action(tx, kind, version)
	})
}

func (s Store) SaveCheckpoint(ctx context.Context, plan computer.Plan, cp computer.Checkpoint) error {
	expectedID, expectedEpoch := plan.Request.CheckpointID, plan.Request.Instance.Epoch
	if plan.Previous != nil {
		expectedID += "-recover"
		expectedEpoch = plan.Previous.Epoch
	}
	if plan.Cleanup != nil {
		expectedID = plan.Cleanup.CheckpointID
	}
	hash, err := computer.Fingerprint(plan.Request.Manifest)
	if err != nil || cp.ID != expectedID || cp.ComputerID != plan.Request.Instance.ComputerID || cp.SourceEpoch != expectedEpoch || cp.ManifestSHA256 != hash {
		return computer.ErrStale
	}
	return s.mutate(ctx, plan, func(tx *sql.Tx, kind string, version int64) error {
		raw, err := encoded(cp)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE computer_operation SET checkpoint_json=? WHERE execution_id=? AND execution_epoch=? AND kind=?`, raw, s.executionID, s.executionEpoch, kind); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE computer SET checkpoint_json=?,version=version+1 WHERE singleton=1 AND version=?`, raw, version)
		if err != nil {
			return err
		}
		if n, e := res.RowsAffected(); e != nil || n != 1 {
			return computer.ErrStale
		}
		return s.host.RecordEvent(ctx, tx, "computer.checkpoint_committed", cp.SourceEpoch)
	})
}

func (s Store) Finish(ctx context.Context, plan computer.Plan) error {
	return s.mutate(ctx, plan, func(tx *sql.Tx, kind string, version int64) error {
		if _, err := tx.ExecContext(ctx, `UPDATE computer_operation SET status='completed' WHERE execution_id=? AND execution_epoch=? AND kind=?`, s.executionID, s.executionEpoch, kind); err != nil {
			return err
		}
		phase := "ready"
		var owner any = s.executionID
		if kind == "idle" {
			phase, owner = "checkpointed", nil
		}
		res, err := tx.ExecContext(ctx, `UPDATE computer SET phase=?,execution_id=?,version=version+1 WHERE singleton=1 AND version=?`, phase, owner, version)
		if err != nil {
			return err
		}
		if n, e := res.RowsAffected(); e != nil || n != 1 {
			return computer.ErrStale
		}
		return s.host.RecordEvent(ctx, tx, "computer."+phase, plan.Request.Instance.Epoch)
	})
}

func (s Store) acceptedPlan(ctx context.Context, tx *sql.Tx, kind string, compatibility computer.Compatibility, manifest computer.RestorationManifest) (computer.Plan, bool, error) {
	var plan computer.Plan
	var request, status string
	var previous, checkpoint sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT request_json,previous_json,checkpoint_json,status FROM computer_operation WHERE execution_id=? AND execution_epoch=? AND kind=?`, s.executionID, s.executionEpoch, kind).Scan(&request, &previous, &checkpoint, &status)
	if err == nil {
		if err := decode(request, &plan.Request); err != nil {
			return plan, true, err
		}
		if !reflect.DeepEqual(plan.Request.Compatibility, compatibility) || !reflect.DeepEqual(plan.Request.Manifest, manifest) {
			return plan, true, computer.ErrCompatibility
		}
		if previous.Valid {
			if err := decode(previous.String, &plan.Previous); err != nil {
				return plan, true, err
			}
		}
		if checkpoint.Valid {
			if err := decode(checkpoint.String, &plan.Checkpoint); err != nil {
				return plan, true, err
			}
		}
		plan.Done = status == "completed"
		if plan.Done && kind == "ensure" {
			var valid bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM computer WHERE execution_id=? AND execution_epoch=? AND instance_epoch=? AND phase='ready')`, s.executionID, s.executionEpoch, plan.Request.Instance.Epoch).Scan(&valid); err != nil {
				return plan, true, err
			}
			if !valid {
				return plan, true, computer.ErrStale
			}
		}
		return plan, true, attachCleanup(ctx, tx, &plan)
	}
	if err != sql.ErrNoRows {
		return plan, false, err
	}
	return plan, false, nil

}

type lifecycleChange struct {
	compatibility    computer.Compatibility
	manifest         computer.RestorationManifest
	kind             string
	instance         computer.Instance
	compatibilityRaw string
	manifestRaw      string
	checkpoint       sql.NullString
	fresh            bool
	version          int64
}

func (s Store) publishPlan(ctx context.Context, tx *sql.Tx, plan *computer.Plan, change lifecycleChange) error {
	var err error
	change.instance.NativeName = "collie-" + change.instance.ComputerID + "-" + strconv.FormatInt(change.instance.Epoch, 10)
	if plan.Request.OperationID == "" {
		plan.Request = computer.Request{Instance: change.instance, OperationID: newID(), CheckpointID: newID(), Compatibility: change.compatibility, Manifest: change.manifest}
	}
	if change.checkpoint.Valid {
		if err := decode(change.checkpoint.String, &plan.Checkpoint); err != nil {
			return err
		}
	}
	instanceRaw, err := encoded(change.instance)
	if err != nil {
		return err
	}
	phase := "restoring"
	if change.kind == "idle" {
		phase = "checkpointing"
	}
	if change.fresh {
		_, err = tx.ExecContext(ctx, `INSERT INTO computer VALUES(1,?,?,?,?,?,?,?, ?,?,1)`, change.instance.ComputerID, change.instance.Epoch, instanceRaw, change.compatibilityRaw, change.manifestRaw, nil, phase, s.executionID, s.executionEpoch)
	} else {
		var res sql.Result
		res, err = tx.ExecContext(ctx, `UPDATE computer SET instance_epoch=?,instance_json=?,manifest_json=?,phase=?,execution_id=?,execution_epoch=?,version=version+1 WHERE singleton=1 AND version=?`, change.instance.Epoch, instanceRaw, change.manifestRaw, phase, s.executionID, s.executionEpoch, change.version)
		if err == nil {
			if n, e := res.RowsAffected(); e != nil || n != 1 {
				return computer.ErrStale
			}
		}
	}
	if err != nil {
		return err
	}
	requestRaw, err := encoded(plan.Request)
	if err != nil {
		return err
	}
	var previousRaw any
	if plan.Previous != nil {
		previousRaw, err = encoded(plan.Previous)
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO computer_operation VALUES(?,?,?,?,?,?,?,'accepted')`, newID(), s.executionID, s.executionEpoch, change.kind, requestRaw, previousRaw, nullable(change.checkpoint.String))
	if err != nil {
		return err
	}
	if err := attachCleanup(ctx, tx, plan); err != nil {
		return err
	}
	return s.host.RecordEvent(ctx, tx, "computer."+phase, change.instance.Epoch)
}
